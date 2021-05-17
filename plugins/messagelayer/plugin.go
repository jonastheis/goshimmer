package messagelayer

import (
	"os"
	"sync"
	"time"

	"github.com/iotaledger/goshimmer/packages/consensus/fcob"
	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"github.com/iotaledger/goshimmer/packages/mana"
	"github.com/iotaledger/goshimmer/packages/shutdown"
	"github.com/iotaledger/goshimmer/packages/tangle"
	"github.com/iotaledger/goshimmer/plugins/autopeering/local"
	"github.com/iotaledger/goshimmer/plugins/database"

	"github.com/cockroachdb/errors"
	"github.com/iotaledger/hive.go/daemon"
	"github.com/iotaledger/hive.go/events"
	"github.com/iotaledger/hive.go/identity"
	"github.com/iotaledger/hive.go/node"
)

const (
	// DefaultAverageNetworkDelay contains the default average time it takes for a network to propagate through gossip.
	DefaultAverageNetworkDelay = 5 * time.Second
)

var (
	// ErrMessageWasNotBookedInTime is returned if a message did not get booked within the defined await time.
	ErrMessageWasNotBookedInTime = errors.New("message could not be booked in time")

	// ErrMessageWasNotIssuedInTime is returned if a message did not get issued within the defined await time.
	ErrMessageWasNotIssuedInTime = errors.New("message could not be issued in time")
)

// region Plugin ///////////////////////////////////////////////////////////////////////////////////////////////////////

var (
	plugin     *node.Plugin
	pluginOnce sync.Once
	syncedOnce sync.Once
)

// Plugin gets the plugin instance.
func Plugin() *node.Plugin {
	pluginOnce.Do(func() {
		plugin = node.NewPlugin("MessageLayer", node.Enabled, configure, run)
	})

	return plugin
}

func configure(plugin *node.Plugin) {
	Tangle().Events.Error.Attach(events.NewClosure(func(err error) {
		plugin.LogError(err)
	}))
	Tangle().Events.Info.Attach(events.NewClosure(func(info string) {
		plugin.LogInfo(info)
	}))

	// Messages created by the node need to pass through the normal flow.
	Tangle().MessageFactory.Events.MessageConstructed.Attach(events.NewClosure(func(message *tangle.Message) {
		Tangle().ProcessGossipMessage(message.Bytes(), local.GetInstance().Peer)
	}))

	Tangle().Storage.Events.MessageStored.Attach(events.NewClosure(func(messageID tangle.MessageID) {
		Tangle().Storage.Message(messageID).Consume(func(message *tangle.Message) {
			Tangle().WeightProvider.Update(message.IssuingTime(), identity.NewID(message.IssuerPublicKey()))
		})
	}))

	Tangle().Parser.Events.MessageRejected.Attach(events.NewClosure(func(ev *tangle.MessageRejectedEvent) {
		plugin.LogInfof("message rejected in parser: %s", ev.Message.ID().Base58())
	}))

	Tangle().FifoScheduler.Events.MessageDiscarded.Attach(events.NewClosure(func(messageID tangle.MessageID) {
		plugin.LogInfof("message discarded in FifoScheduler %s", messageID.Base58())
	}))

	Tangle().FifoScheduler.Events.NodeBlacklisted.Attach(events.NewClosure(func(nodeID identity.ID) {
		plugin.LogInfof("node %s is blacklisted in FifoScheduler", nodeID.String())
	}))

	Tangle().FifoScheduler.Events.MessageScheduled.Attach(events.NewClosure(func(messageID tangle.MessageID) {
		plugin.LogDebugf("message scheduled in FifoScheduler: %s", messageID.Base58())
	}))

	Tangle().RateSetter.Events.MessageDiscarded.Attach(events.NewClosure(func(messageID tangle.MessageID) {
		plugin.LogInfof("message discarded in rate setter: %s", messageID.Base58())
	}))

	Tangle().Scheduler.Events.MessageDiscarded.Attach(events.NewClosure(func(messageID tangle.MessageID) {
		plugin.LogInfof("message rejected in scheduler: %s", messageID.Base58())
	}))

	Tangle().Scheduler.Events.NodeBlacklisted.Attach(events.NewClosure(func(nodeID identity.ID) {
		plugin.LogInfof("node %s is blacklisted in scheduler", nodeID.String())
	}))

	Tangle().Scheduler.Events.MessageScheduled.Attach(events.NewClosure(func(messageID tangle.MessageID) {
		plugin.LogDebugf("message scheduled in scheduler: %s", messageID.Base58())
	}))

	Tangle().Events.MessageInvalid.Attach(events.NewClosure(func(messageID tangle.MessageID) {
		plugin.LogInfof("message invalid: %s", messageID.Base58())
	}))

	Tangle().Booker.Events.MessageBooked.Attach(events.NewClosure(func(messageID tangle.MessageID) {
		plugin.LogDebugf("message booked in message layer: %s", messageID.Base58())
	}))

	Tangle().Events.SyncChanged.Attach(events.NewClosure(func(ev *tangle.SyncChangedEvent) {
		plugin.LogInfo("Sync changed: ", ev.Synced)
		if ev.Synced {
			// Only for the first synced
			syncedOnce.Do(func() {
				Tangle().Scheduler.Setup()        // start buffering solid messages
				Tangle().FifoScheduler.Detach()   // stop receiving more messages
				Tangle().FifoScheduler.Shutdown() // schedule remaining messages
				Tangle().Scheduler.Start()        // start the actual scheduler
			})
			// make sure that we are using the configured rate when synced
			rate := Tangle().Options.SchedulerParams.Rate
			Tangle().Scheduler.SetRate(rate)
			plugin.LogInfof("Scheduler rate: %v", rate)
		} else {
			// increase scheduler rate
			rate := Tangle().Options.SchedulerParams.Rate
			rate -= rate / 2 // 50% increase
			Tangle().Scheduler.SetRate(rate)
			plugin.LogInfof("Scheduler rate: %v", rate)
		}
	}))

	// read snapshot file
	if Parameters.Snapshot.File != "" {
		snapshot := &ledgerstate.Snapshot{}
		f, err := os.Open(Parameters.Snapshot.File)
		if err != nil {
			plugin.Panic("can not open snapshot file:", err)
		}
		if _, err := snapshot.ReadFrom(f); err != nil {
			plugin.Panic("could not read snapshot file:", err)
		}
		Tangle().LedgerState.LoadSnapshot(snapshot)
		plugin.LogInfof("read snapshot from %s", Parameters.Snapshot.File)
	}

	fcob.LikedThreshold = time.Duration(Parameters.FCOB.AverageNetworkDelay) * time.Second
	fcob.LocallyFinalizedThreshold = time.Duration(Parameters.FCOB.AverageNetworkDelay*2) * time.Second

	configureApprovalWeight()
}

func run(*node.Plugin) {
	if err := daemon.BackgroundWorker("Tangle", func(shutdownSignal <-chan struct{}) {
		syncCheckTimer := time.NewTicker(time.Second)
		sync := false
		// hack of switching scheduler
	out:
		for {
			select {
			case <-syncCheckTimer.C:
				newSync := Tangle().TimeManager.Synced()
				if newSync != sync {
					go Tangle().Events.SyncChanged.Trigger(&tangle.SyncChangedEvent{Synced: newSync})
					sync = newSync
				}
			case <-shutdownSignal:
				break out
			}
		}
		Tangle().Shutdown()
	}, shutdown.PriorityTangle); err != nil {
		plugin.Panicf("Failed to start as daemon: %s", err)
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Tangle ///////////////////////////////////////////////////////////////////////////////////////////////////////

var (
	tangleInstance *tangle.Tangle
	tangleOnce     sync.Once
)

// Tangle gets the tangle instance.
func Tangle() *tangle.Tangle {
	tangleOnce.Do(func() {
		tangleInstance = tangle.New(
			tangle.Store(database.Store()),
			tangle.Identity(local.GetInstance().LocalIdentity()),
			tangle.Width(Parameters.TangleWidth),
			tangle.Consensus(ConsensusMechanism()),
			tangle.GenesisNode(Parameters.Snapshot.GenesisNode),
			tangle.SchedulerConfig(tangle.SchedulerParams{
				Rate:                        schedulerRate(SchedulerParameters.Rate),
				AccessManaRetrieveFunc:      accessManaRetriever,
				TotalAccessManaRetrieveFunc: totalAccessManaRetriever,
			}),
			tangle.RateSetterConfig(tangle.RateSetterParams{
				Initial: &RateSetterParameters.Initial,
			}),
			tangle.SyncTimeWindow(Parameters.TangleTimeWindow),
			tangle.StartSynced(Parameters.StartSynced),
		)

		tangleInstance.WeightProvider = tangle.NewCManaWeightProvider(GetCMana, tangleInstance.TimeManager.Time, database.Store())

		tangleInstance.Setup()
	})
	return tangleInstance
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region ConsensusMechanism ///////////////////////////////////////////////////////////////////////////////////////////

var (
	consensusMechanism     *fcob.ConsensusMechanism
	consensusMechanismOnce sync.Once
)

// ConsensusMechanism return the FcoB ConsensusMechanism used by the Tangle.
func ConsensusMechanism() *fcob.ConsensusMechanism {
	consensusMechanismOnce.Do(func() {
		consensusMechanism = fcob.NewConsensusMechanism()
	})

	return consensusMechanism
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Scheduler ///////////////////////////////////////////////////////////////////////////////////////////

func schedulerRate(durationString string) time.Duration {
	duration, err := time.ParseDuration(durationString)
	// if parseDuration failed, scheduler will take default value (5ms)
	if err != nil {
		return 0
	}
	return duration
}

func accessManaRetriever(nodeID identity.ID) float64 {
	nodeMana, _, err := GetAccessMana(nodeID)
	if err != nil {
		return 0
	}
	return nodeMana
}

func totalAccessManaRetriever() float64 {
	totalMana, _, err := GetTotalMana(mana.AccessMana)
	if err != nil {
		return 0
	}
	return totalMana
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// AwaitMessageToBeBooked awaits maxAwait for the given message to get booked.
func AwaitMessageToBeBooked(f func() (*tangle.Message, error), txID ledgerstate.TransactionID, maxAwait time.Duration) (*tangle.Message, error) {
	// first subscribe to the transaction booked event
	booked := make(chan struct{}, 1)
	// exit is used to let the caller exit if for whatever
	// reason the same transaction gets booked multiple times
	exit := make(chan struct{})
	defer close(exit)

	closure := events.NewClosure(func(msgID tangle.MessageID) {
		match := false
		Tangle().Storage.Message(msgID).Consume(func(message *tangle.Message) {
			if message.Payload().Type() == ledgerstate.TransactionType {
				tx := message.Payload().(*ledgerstate.Transaction)
				if tx.ID() == txID {
					match = true
					return
				}
			}
		})
		if !match {
			return
		}
		select {
		case booked <- struct{}{}:
		case <-exit:
		}
	})
	Tangle().Booker.Events.MessageBooked.Attach(closure)
	defer Tangle().Booker.Events.MessageBooked.Detach(closure)

	// then issue the message with the tx

	// channel to receive the result of issuance
	issueResult := make(chan struct {
		msg *tangle.Message
		err error
	}, 1)

	go func() {
		msg, err := f()
		issueResult <- struct {
			msg *tangle.Message
			err error
		}{msg: msg, err: err}
	}()

	// wait on issuance
	result := <-issueResult

	if result.err != nil || result.msg == nil {
		return nil, errors.Errorf("Failed to issue transaction %s: %w", txID.String(), result.err)
	}

	select {
	case <-time.After(maxAwait):
		return nil, ErrMessageWasNotBookedInTime
	case <-booked:
		return result.msg, nil
	}
}

// AwaitMessageToBeIssued awaits maxAwait for the given message to get issued.
func AwaitMessageToBeIssued(messageID tangle.MessageID, maxAwait time.Duration) error {
	issued := make(chan struct{}, 1)
	exit := make(chan struct{})
	defer close(exit)

	closure := events.NewClosure(func(message *tangle.Message) {
		if message.ID() != messageID {
			return
		}
		select {
		case issued <- struct{}{}:
		case <-exit:
		}
	})
	Tangle().Scheduler.Events.MessageScheduled.Attach(closure)
	defer Tangle().Scheduler.Events.MessageScheduled.Detach(closure)

	select {
	case <-time.After(maxAwait):
		return ErrMessageWasNotIssuedInTime
	case <-issued:
		return nil
	}
}

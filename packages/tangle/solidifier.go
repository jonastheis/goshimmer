package tangle

import (
	"time"

	"github.com/iotaledger/hive.go/events"
)

// maxParentsTimeDifference defines the smallest allowed time difference between a child Message and its parents.
const minParentsTimeDifference = 0 * time.Second

// maxParentsTimeDifference defines the biggest allowed time difference between a child Message and its parents.
const maxParentsTimeDifference = 30 * time.Minute

// region Solidifier ///////////////////////////////////////////////////////////////////////////////////////////////////

// Solidifier is the Tangle's component that solidifies messages.
type Solidifier struct {
	// Events contains the Solidifier related events.
	Events *SolidifierEvents

	tangle *Tangle
}

// NewSolidifier is the constructor of the Solidifier.
func NewSolidifier(tangle *Tangle) (solidifier *Solidifier) {
	solidifier = &Solidifier{
		Events: &SolidifierEvents{
			MessageSolid: events.NewEvent(messageIDEventHandler),
		},

		tangle: tangle,
	}

	return
}

// Solidify solidifies the given Message.
func (s *Solidifier) Solidify(messageID MessageID) {
	s.tangle.WalkMessages(s.checkMessageSolidity, MessageIDs{messageID}, true)
}

// checkMessageSolidity checks if the given Message is solid and eventually queues its Approvers to also be checked.
func (s *Solidifier) checkMessageSolidity(message *Message, messageMetadata *MessageMetadata) (nextMessagesToCheck MessageIDs) {
	if !s.isMessageSolid(message, messageMetadata) {
		return
	}

	if !s.areParentMessagesValid(message) {
		if !messageMetadata.SetInvalid(true) {
			return
		}
		s.tangle.Events.MessageInvalid.Trigger(message.ID())
		return
	}

	if !messageMetadata.SetSolid(true) {
		return
	}
	s.Events.MessageSolid.Trigger(message.ID())

	s.tangle.Storage.Approvers(message.ID()).Consume(func(approver *Approver) {
		nextMessagesToCheck = append(nextMessagesToCheck, approver.ApproverMessageID())
	})

	return
}

// isMessageSolid checks if the given Message is solid.
func (s *Solidifier) isMessageSolid(message *Message, messageMetadata *MessageMetadata) (solid bool) {
	if message == nil || message.IsDeleted() || messageMetadata == nil || messageMetadata.IsDeleted() {
		return false
	}

	if messageMetadata.IsSolid() {
		return true
	}

	solid = true
	message.ForEachParent(func(parent Parent) {
		// as missing messages are requested in isMessageMarkedAsSolid, we need to be aware of short-circuit evaluation
		// rules, thus we need to evaluate isMessageMarkedAsSolid !!first!!
		solid = s.isMessageMarkedAsSolid(parent.ID) && solid
	})

	return
}

// isMessageMarkedAsSolid checks whether the given message is solid and marks it as missing if it isn't known.
func (s *Solidifier) isMessageMarkedAsSolid(messageID MessageID) (solid bool) {
	if messageID == EmptyMessageID {
		return true
	}

	s.tangle.Storage.StoreIfMissingMessageMetadata(messageID).Consume(func(messageMetadata *MessageMetadata) {
		solid = messageMetadata.IsSolid()
	})

	return
}

// areParentMessagesValid checks whether the parents of the given Message are valid.
func (s *Solidifier) areParentMessagesValid(message *Message) (valid bool) {
	valid = true
	message.ForEachParent(func(parent Parent) {
		valid = valid && s.isParentMessageValid(parent.ID, message.IssuingTime())
	})

	return
}

// isParentMessageValid checks whether the given parent Message is valid.
func (s *Solidifier) isParentMessageValid(parentMessageID MessageID, childMessageIssuingTime time.Time) (valid bool) {
	if parentMessageID == EmptyMessageID {
		return true
	}

	s.tangle.Storage.Message(parentMessageID).Consume(func(parentMessage *Message) {
		timeDifference := childMessageIssuingTime.Sub(parentMessage.IssuingTime())

		valid = timeDifference >= minParentsTimeDifference && timeDifference <= maxParentsTimeDifference
	})

	s.tangle.Storage.MessageMetadata(parentMessageID).Consume(func(messageMetadata *MessageMetadata) {
		valid = valid && !messageMetadata.IsInvalid()
	})

	return
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region SolidifierEvents /////////////////////////////////////////////////////////////////////////////////////////////

// SolidifierEvents represents events happening in the Solidifier.
type SolidifierEvents struct {
	// MessageSolid is triggered when a message becomes solid, i.e. its past cone is known and solid.
	MessageSolid *events.Event
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

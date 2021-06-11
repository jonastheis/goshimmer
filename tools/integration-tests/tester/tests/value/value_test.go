package value

import (
	"testing"
	"time"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/identity"
	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/goshimmer/client/wallet"
	"github.com/iotaledger/goshimmer/client/wallet/packages/address"
	"github.com/iotaledger/goshimmer/client/wallet/packages/createnftoptions"
	"github.com/iotaledger/goshimmer/client/wallet/packages/delegateoptions"
	"github.com/iotaledger/goshimmer/client/wallet/packages/destroynftoptions"
	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"github.com/iotaledger/goshimmer/tools/integration-tests/tester/framework"
	"github.com/iotaledger/goshimmer/tools/integration-tests/tester/tests"
)

var maxAwaitDuration = 30 * time.Second

// TestTransactionPersistence issues messages on random peers, restarts them and checks for persistence after restart.
func TestTransactionPersistence(t *testing.T) {
	n, err := f.CreateNetwork("transaction_TestPersistence", 4, framework.CreateNetworkConfig{Faucet: true, StartSynced: true})
	require.NoError(t, err)
	defer tests.ShutdownNetwork(t, n)

	// request funds from faucet
	_, addrBalance := tests.SendFaucetRequestOnAllPeers(t, n.Peers())

	// wait for messages to be gossiped
	err = tests.AwaitPeerGetFundsFromFaucet(n.Peers(), maxAwaitDuration)
	require.NoError(t, err)

	// check ledger state
	tests.CheckBalances(t, n.Peers(), addrBalance)

	// send transaction randomly
	txIds := make(map[string]tests.ExpectedInclusionState)
	randomTxIds := tests.SendTransactionOnRandomPeer(t, n.Peers(), addrBalance, 10, 100)
	for _, randomTxId := range randomTxIds {
		txIds[randomTxId] = tests.ExpectedInclusionState{
			Confirmed: tests.True(),
		}
	}

	err = tests.AwaitTransactionInclusionState(n.Peers(), txIds, maxAwaitDuration)
	require.NoError(t, err)

	// check ledger state
	tests.CheckBalances(t, n.Peers(), addrBalance)

	// 3. stop all nodes
	for _, peer := range n.Peers()[1:] {
		err = peer.Stop()
		require.NoError(t, err)
	}

	// 4. start all nodes
	for _, peer := range n.Peers()[1:] {
		err = peer.Start()
		require.NoError(t, err)
	}

	// wait for peers to start
	time.Sleep(5 * time.Second)
	err = n.DoManualPeeringAndWait()
	require.NoError(t, err)

	err = tests.AwaitTransactionInclusionState(n.Peers(), txIds, maxAwaitDuration)
	require.NoError(t, err)

	// 5. check ledger state
	tests.CheckBalances(t, n.Peers(), addrBalance)
}

// TestValueColoredPersistence issues colored tokens on random peers, restarts them and checks for persistence after restart.
func TestValueColoredPersistence(t *testing.T) {
	n, err := f.CreateNetwork("valueColor_TestPersistence", 4, framework.CreateNetworkConfig{Faucet: true, StartSynced: true})
	require.NoError(t, err)
	defer tests.ShutdownNetwork(t, n)

	// request funds from faucet
	_, addrBalance := tests.SendFaucetRequestOnAllPeers(t, n.Peers())
	// wait for messages to be gossiped
	tests.AwaitPeerGetFundsFromFaucet(n.Peers(), maxAwaitDuration)

	// check ledger state
	tests.CheckBalances(t, n.Peers(), addrBalance)

	// send colored funds to faucet
	txIds := make(map[string]tests.ExpectedInclusionState)
	for _, peer := range n.Peers()[1:] {
		fail, txId := tests.SendColoredTransaction(t, peer, n.Peers()[0], addrBalance, tests.TransactionConfig{})
		require.False(t, fail)
		txIds[txId] = tests.ExpectedInclusionState{
			Confirmed: tests.True(),
		}
	}

	// check whether all issued transactions are persistently available on all nodes, and confirmed
	err = tests.AwaitTransactionInclusionState(n.Peers(), txIds, maxAwaitDuration)
	require.NoError(t, err)

	// check ledger state
	tests.CheckBalances(t, n.Peers(), addrBalance)

	// stop all nodes
	for _, peer := range n.Peers()[1:] {
		err = peer.Stop()
		require.NoError(t, err)
	}

	// start all nodes
	for _, peer := range n.Peers()[1:] {
		err = peer.Start()
		require.NoError(t, err)
	}

	// wait for peers to start
	time.Sleep(5 * time.Second)
	err = n.DoManualPeeringAndWait()
	require.NoError(t, err)

	// check whether all issued transactions are persistently available on all nodes, and confirmed
	err = tests.AwaitTransactionInclusionState(n.Peers(), txIds, maxAwaitDuration)
	require.NoError(t, err)

	// 5. check ledger state
	tests.CheckBalances(t, n.Peers(), addrBalance)
}

// TestAlias_Persistence creates an alias output, restarts all nodes, and checks whether the output is persisted.
func TestAlias_Persistence(t *testing.T) {
	n, err := f.CreateNetwork("alias_TestPersistence", 4, framework.CreateNetworkConfig{Faucet: true, StartSynced: true})
	require.NoError(t, err)
	defer tests.ShutdownNetwork(t, n)

	// create a wallet that connects to a random peer
	w := wallet.New(wallet.WebAPI(n.Peers()[1].BaseURL()), wallet.FaucetPowDifficulty(framework.ParaPoWFaucetDifficulty))

	err = w.RequestFaucetFunds(true)
	require.NoError(t, err)

	tx, aliasID, err := w.CreateNFT(
		createnftoptions.ImmutableData([]byte("can't touch this")),
		createnftoptions.WaitForConfirmation(true),
	)
	require.NoError(t, err)

	err = tests.AwaitTransactionInclusionState(n.Peers(), map[string]tests.ExpectedInclusionState{
		tx.ID().Base58(): {
			Confirmed: tests.True(),
			Rejected:  tests.False(),
		},
	}, maxAwaitDuration)
	require.NoError(t, err)

	aliasOutputID := checkAliasOutputOnAllPeers(t, n.Peers(), aliasID)

	// stop all nodes
	for _, peer := range n.Peers()[1:] {
		err = peer.Stop()
		require.NoError(t, err)
	}

	// start all nodes
	for _, peer := range n.Peers()[1:] {
		err = peer.Start()
		require.NoError(t, err)
	}

	// wait for peers to start
	time.Sleep(5 * time.Second)
	err = n.DoManualPeeringAndWait()
	require.NoError(t, err)

	// check if nodes still have the outputs and transaction
	err = tests.AwaitTransactionInclusionState(n.Peers(), map[string]tests.ExpectedInclusionState{
		tx.ID().Base58(): {
			Confirmed: tests.True(),
			Rejected:  tests.False(),
		},
	}, maxAwaitDuration)
	require.NoError(t, err)

	checkAliasOutputOnAllPeers(t, n.Peers(), aliasID)

	_, err = w.DestroyNFT(destroynftoptions.Alias(aliasID.Base58()), destroynftoptions.WaitForConfirmation(true))
	require.NoError(t, err)
	// give enough time to all peers
	time.Sleep(2 * time.Second)

	// check if all nodes destroyed it
	for _, peer := range n.Peers() {
		outputMetadata, err := peer.GetOutputMetadata(aliasOutputID.Base58())
		require.NoError(t, err)
		// it has been spent
		require.True(t, outputMetadata.ConsumerCount > 0)

		resp, err := peer.GetAddressUnspentOutputs(aliasID.Base58())
		require.NoError(t, err)
		// there should be no outputs
		require.True(t, len(resp.Outputs) == 0)
	}
}

// TestAlias_Delegation tests if a delegation output can be used to refresh mana.
func TestAlias_Delegation(t *testing.T) {
	n, err := f.CreateNetwork("alias_TestDelegation", 4, framework.CreateNetworkConfig{Faucet: true, StartSynced: true})
	require.NoError(t, err)
	defer tests.ShutdownNetwork(t, n)

	// create a wallet that connects to a random peer
	w := wallet.New(wallet.WebAPI(n.RandomPeer().BaseURL()), wallet.FaucetPowDifficulty(framework.ParaPoWFaucetDifficulty))

	err = w.RequestFaucetFunds(true)
	require.NoError(t, err)

	dumbWallet := createWallets(1)[0]
	delegationAddress := dumbWallet.address
	_, delegationIDs, err := w.DelegateFunds(
		delegateoptions.Destination(address.Address{AddressBytes: delegationAddress.Array()}, map[ledgerstate.Color]uint64{ledgerstate.ColorIOTA: 1000}),
		delegateoptions.WaitForConfirmation(true),
	)
	require.NoError(t, err)
	// give enough time to all peers
	time.Sleep(2 * time.Second)

	delegatedAliasOutputID := ledgerstate.OutputID{}
	delegatedAliasOutput := &ledgerstate.AliasOutput{}
	for i, peer := range n.Peers() {
		resp, err := peer.GetAddressUnspentOutputs(delegationIDs[0].Base58())
		require.NoError(t, err)
		// there should be only this output
		require.True(t, len(resp.Outputs) == 1)
		shouldBeAliasOutput, err := resp.Outputs[0].ToLedgerstateOutput()
		require.NoError(t, err)
		require.Equal(t, ledgerstate.AliasOutputType, shouldBeAliasOutput.Type())
		alias, ok := shouldBeAliasOutput.(*ledgerstate.AliasOutput)
		require.True(t, ok)
		require.Equal(t, delegationIDs[0].Base58(), alias.GetAliasAddress().Base58())
		require.True(t, alias.IsDelegated())
		switch i {
		case 0:
			delegatedAliasOutputID = alias.ID()
			delegatedAliasOutput = alias
		default:
			require.Equal(t, delegatedAliasOutputID.Base58(), alias.ID().Base58())
			require.Equal(t, delegatedAliasOutput.Bytes(), alias.Bytes())
		}
	}

	aManaReceiver, err := identity.RandomID()
	require.NoError(t, err)
	cManaReceiver, err := identity.RandomID()
	require.NoError(t, err)

	// let's try to "refresh mana"
	nextOutput := delegatedAliasOutput.NewAliasOutputNext(false)
	essence := ledgerstate.NewTransactionEssence(0, time.Now(),
		aManaReceiver, cManaReceiver,
		ledgerstate.NewInputs(ledgerstate.NewUTXOInput(delegatedAliasOutputID)),
		ledgerstate.NewOutputs(nextOutput))
	tx := ledgerstate.NewTransaction(essence, dumbWallet.unlockBlocks(essence))
	_, err = n.RandomPeer().PostTransaction(tx.Bytes())
	require.NoError(t, err)

	err = tests.AwaitTransactionInclusionState(n.Peers(), map[string]tests.ExpectedInclusionState{
		tx.ID().Base58(): {
			Confirmed: tests.True(),
		},
	}, maxAwaitDuration)

	aManaReceiverCurrMana, err := n.RandomPeer().GetManaFullNodeID(base58.Encode(aManaReceiver.Bytes()))
	require.NoError(t, err)
	cManaReceiverCurrMana, err := n.RandomPeer().GetManaFullNodeID(base58.Encode(cManaReceiver.Bytes()))
	require.NoError(t, err)

	// check that the pledge actually worked
	require.True(t, aManaReceiverCurrMana.Access > 0)
	require.True(t, cManaReceiverCurrMana.Consensus > 0)
}

func checkAliasOutputOnAllPeers(t *testing.T, peers []*framework.Peer, aliasAddr *ledgerstate.AliasAddress) ledgerstate.OutputID {
	aliasOutputID := ledgerstate.OutputID{}

	for i, peer := range peers {
		resp, err := peer.GetAddressUnspentOutputs(aliasAddr.Base58())
		require.NoError(t, err)
		// there should be only this output
		require.True(t, len(resp.Outputs) == 1)
		shouldBeAliasOutput, err := resp.Outputs[0].ToLedgerstateOutput()
		require.NoError(t, err)
		require.Equal(t, ledgerstate.AliasOutputType, shouldBeAliasOutput.Type())
		alias, ok := shouldBeAliasOutput.(*ledgerstate.AliasOutput)
		require.True(t, ok)
		require.Equal(t, aliasAddr.Base58(), alias.GetAliasAddress().Base58())
		switch i {
		case 0:
			aliasOutputID = alias.ID()
		default:
			require.Equal(t, aliasOutputID.Base58(), alias.ID().Base58())
		}
	}
	return aliasOutputID
}

type simpleWallet struct {
	keyPair ed25519.KeyPair
	address *ledgerstate.ED25519Address
}

func (s simpleWallet) privateKey() ed25519.PrivateKey {
	return s.keyPair.PrivateKey
}

func (s simpleWallet) publicKey() ed25519.PublicKey {
	return s.keyPair.PublicKey
}

func createWallets(n int) []simpleWallet {
	wallets := make([]simpleWallet, n)
	for i := 0; i < n; i++ {
		kp := ed25519.GenerateKeyPair()
		wallets[i] = simpleWallet{
			kp,
			ledgerstate.NewED25519Address(kp.PublicKey),
		}
	}
	return wallets
}

func (s simpleWallet) sign(txEssence *ledgerstate.TransactionEssence) *ledgerstate.ED25519Signature {
	return ledgerstate.NewED25519Signature(s.publicKey(), s.privateKey().Sign(txEssence.Bytes()))
}

func (s simpleWallet) unlockBlocks(txEssence *ledgerstate.TransactionEssence) []ledgerstate.UnlockBlock {
	unlockBlock := ledgerstate.NewSignatureUnlockBlock(s.sign(txEssence))
	unlockBlocks := make([]ledgerstate.UnlockBlock, len(txEssence.Inputs()))
	for i := range txEssence.Inputs() {
		unlockBlocks[i] = unlockBlock
	}
	return unlockBlocks
}

package wallet

import (
	walletaddr "github.com/iotaledger/goshimmer/client/wallet/packages/address"
	"github.com/iotaledger/goshimmer/packages/ledgerstate"
)

// UnspentOutputManager is a manager for the unspent outputs of the addresses of a wallet. It allows us to keep track of
// the spent state of outputs using our local knowledge about outputs that have already been spent and allows us to
// cache results that would otherwise have to be requested by the server over and over again.
type UnspentOutputManager struct {
	addressManager *AddressManager
	connector      Connector
	unspentOutputs map[string]map[ledgerstate.TransactionID]*Output
}

// NewUnspentOutputManager creates a new UnspentOutputManager.
func NewUnspentOutputManager(addressManager *AddressManager, connector Connector) (outputManager *UnspentOutputManager) {
	outputManager = &UnspentOutputManager{
		addressManager: addressManager,
		connector:      connector,
		unspentOutputs: make(map[string]map[ledgerstate.TransactionID]*Output),
	}

	outputManager.Refresh(true)

	return
}

// Refresh retrieves the unspent outputs from the node. If includeSpentAddresses is set to true, then it also scans the
// addresses from which we previously spent already.
func (unspentOutputManager *UnspentOutputManager) Refresh(includeSpentAddresses ...bool) (err error) {
	var addressesToRefresh []walletaddr.Address
	if len(includeSpentAddresses) >= 1 && includeSpentAddresses[0] {
		addressesToRefresh = unspentOutputManager.addressManager.Addresses()
	} else {
		addressesToRefresh = unspentOutputManager.addressManager.UnspentAddresses()
	}

	unspentOutputs, err := unspentOutputManager.connector.UnspentOutputs(addressesToRefresh...)
	if err != nil {
		return
	}

	for addr, unspentOutputs := range unspentOutputs {
		for transactionID, output := range unspentOutputs {
			if _, addressExists := unspentOutputManager.unspentOutputs[addr.Base58()]; !addressExists {
				unspentOutputManager.unspentOutputs[addr.Base58()] = make(map[ledgerstate.TransactionID]*Output)
			}

			// mark the output as spent if we already marked it as spent locally
			if existingOutput, outputExists := unspentOutputManager.unspentOutputs[addr.Base58()][transactionID]; outputExists && existingOutput.InclusionState.Spent {
				output.InclusionState.Spent = true
			}

			unspentOutputManager.unspentOutputs[addr.Base58()][transactionID] = output
		}
	}

	return
}

// UnspentOutputs returns the outputs that have not been spent, yet.
func (unspentOutputManager *UnspentOutputManager) UnspentOutputs(addresses ...walletaddr.Address) (unspentOutputs map[walletaddr.Address]map[ledgerstate.TransactionID]*Output) {
	// prepare result
	unspentOutputs = make(map[walletaddr.Address]map[ledgerstate.TransactionID]*Output)

	// retrieve the list of addresses from the address manager if none was provided
	if len(addresses) == 0 {
		addresses = unspentOutputManager.addressManager.Addresses()
	}

	// iterate through addresses and scan for unspent outputs
	for _, addr := range addresses {
		// skip the address if we have no outputs for it stored
		unspentOutputsOnAddress, addressExistsInStoredOutputs := unspentOutputManager.unspentOutputs[addr.Base58()]
		if !addressExistsInStoredOutputs {
			continue
		}

		// iterate through outputs
		for transactionID, output := range unspentOutputsOnAddress {
			// skip spent outputs
			if output.InclusionState.Spent {
				continue
			}

			// store unspent outputs in result
			if _, addressExists := unspentOutputs[addr]; !addressExists {
				unspentOutputs[addr] = make(map[ledgerstate.TransactionID]*Output)
			}
			unspentOutputs[addr][transactionID] = output
		}
	}

	return
}

// MarkOutputSpent marks the output identified by the given parameters as spent.
func (unspentOutputManager *UnspentOutputManager) MarkOutputSpent(addr walletaddr.Address, transactionID ledgerstate.TransactionID) {
	// abort if we try to mark an unknown output as spent
	if _, addressExists := unspentOutputManager.unspentOutputs[addr.Base58()]; !addressExists {
		return
	}
	output, outputExists := unspentOutputManager.unspentOutputs[addr.Base58()][transactionID]
	if !outputExists {
		return
	}

	// mark output as spent
	output.InclusionState.Spent = true
}

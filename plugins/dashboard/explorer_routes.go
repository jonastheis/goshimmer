package dashboard

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo"
	"github.com/mr-tron/base58/base58"

	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"github.com/iotaledger/goshimmer/packages/tangle"
	"github.com/iotaledger/goshimmer/plugins/messagelayer"
	"github.com/iotaledger/goshimmer/plugins/webapi/jsonmodels"
	"github.com/iotaledger/goshimmer/plugins/webapi/jsonmodels/value"
	ledgerstateAPI "github.com/iotaledger/goshimmer/plugins/webapi/ledgerstate"
	manaAPI "github.com/iotaledger/goshimmer/plugins/webapi/mana"
)

// ExplorerMessage defines the struct of the ExplorerMessage.
type ExplorerMessage struct {
	// ID is the message ID.
	ID string `json:"id"`
	// SolidificationTimestamp is the timestamp of the message.
	SolidificationTimestamp int64 `json:"solidification_timestamp"`
	// The time when this message was issued
	IssuanceTimestamp int64 `json:"issuance_timestamp"`
	// The issuer's sequence number of this message.
	SequenceNumber uint64 `json:"sequence_number"`
	// The public key of the issuer who issued this message.
	IssuerPublicKey string `json:"issuer_public_key"`
	// The signature of the message.
	Signature string `json:"signature"`
	// StrongParents are the strong parents (references) of the message.
	StrongParents []string `json:"strongParents"`
	// WeakParents are the weak parents (references) of the message.
	WeakParents []string `json:"weakParents"`
	// StrongApprovers are the strong approvers of the message.
	StrongApprovers []string `json:"strongApprovers"`
	// WeakApprovers are the weak approvers of the message.
	WeakApprovers []string `json:"weakApprovers"`
	// Solid defines the solid status of the message.
	Solid     bool   `json:"solid"`
	BranchID  string `json:"branchID"`
	Scheduled bool   `json:"scheduled"`
	Booked    bool   `json:"booked"`
	Eligible  bool   `json:"eligible"`
	Invalid   bool   `json:"invalid"`
	// PayloadType defines the type of the payload.
	PayloadType uint32 `json:"payload_type"`
	// Payload is the content of the payload.
	Payload interface{} `json:"payload"`
}

func createExplorerMessage(msg *tangle.Message) *ExplorerMessage {
	messageID := msg.ID()
	cachedMessageMetadata := messagelayer.Tangle().Storage.MessageMetadata(messageID)
	defer cachedMessageMetadata.Release()
	messageMetadata := cachedMessageMetadata.Unwrap()
	t := &ExplorerMessage{
		ID:                      messageID.Base58(),
		SolidificationTimestamp: messageMetadata.SolidificationTime().Unix(),
		IssuanceTimestamp:       msg.IssuingTime().Unix(),
		IssuerPublicKey:         msg.IssuerPublicKey().String(),
		Signature:               msg.Signature().String(),
		SequenceNumber:          msg.SequenceNumber(),
		StrongParents:           msg.StrongParents().ToStrings(),
		WeakParents:             msg.WeakParents().ToStrings(),
		StrongApprovers:         messagelayer.Tangle().Utils.ApprovingMessageIDs(messageID, tangle.StrongApprover).ToStrings(),
		WeakApprovers:           messagelayer.Tangle().Utils.ApprovingMessageIDs(messageID, tangle.WeakApprover).ToStrings(),
		Solid:                   messageMetadata.IsSolid(),
		BranchID:                messageMetadata.BranchID().Base58(),
		Scheduled:               messageMetadata.Scheduled(),
		Booked:                  messageMetadata.IsBooked(),
		Eligible:                messageMetadata.IsEligible(),
		Invalid:                 messageMetadata.IsInvalid(),
		PayloadType:             uint32(msg.Payload().Type()),
		Payload:                 ProcessPayload(msg.Payload()),
	}

	return t
}

// ExplorerAddress defines the struct of the ExplorerAddress.
type ExplorerAddress struct {
	Address         string           `json:"address"`
	ExplorerOutputs []ExplorerOutput `json:"explorerOutputs"`
}

// ExplorerOutput defines the struct of the ExplorerOutput.
type ExplorerOutput struct {
	ID             *jsonmodels.OutputID       `json:"id"`
	Output         *jsonmodels.Output         `json:"output"`
	Metadata       *jsonmodels.OutputMetadata `json:"metadata"`
	InclusionState value.InclusionState       `json:"inclusionState"`
	TxTimestamp    int                        `json:"txTimestamp"`
	PendingMana    float64                    `json:"pendingMana"`
}

// SearchResult defines the struct of the SearchResult.
type SearchResult struct {
	// Message is the *ExplorerMessage.
	Message *ExplorerMessage `json:"message"`
	// Address is the *ExplorerAddress.
	Address *ExplorerAddress `json:"address"`
}

func setupExplorerRoutes(routeGroup *echo.Group) {
	routeGroup.GET("/message/:id", func(c echo.Context) (err error) {
		messageID, err := tangle.NewMessageID(c.Param("id"))
		if err != nil {
			return
		}

		t, err := findMessage(messageID)
		if err != nil {
			return
		}

		return c.JSON(http.StatusOK, t)
	})

	routeGroup.GET("/address/:id", func(c echo.Context) error {
		addr, err := findAddress(c.Param("id"))
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, addr)
	})

	routeGroup.GET("/transaction/:transactionID", ledgerstateAPI.GetTransaction)
	routeGroup.GET("/transaction/:transactionID/metadata", ledgerstateAPI.GetTransactionMetadata)
	routeGroup.GET("/transaction/:transactionID/attachments", ledgerstateAPI.GetTransactionAttachments)
	routeGroup.GET("/output/:outputID", ledgerstateAPI.GetOutput)
	routeGroup.GET("/output/:outputID/metadata", ledgerstateAPI.GetOutputMetadata)
	routeGroup.GET("/output/:outputID/consumers", ledgerstateAPI.GetOutputConsumers)
	routeGroup.GET("/mana/pending", manaAPI.GetPendingMana)
	routeGroup.GET("/branch/:branchID", ledgerstateAPI.GetBranch)
	routeGroup.GET("/branch/:branchID/children", ledgerstateAPI.GetBranchChildren)
	routeGroup.GET("/branch/:branchID/conflicts", ledgerstateAPI.GetBranchConflicts)

	routeGroup.GET("/search/:search", func(c echo.Context) error {
		search := c.Param("search")
		result := &SearchResult{}

		searchInByte, err := base58.Decode(search)
		if err != nil {
			return fmt.Errorf("%w: search ID %s", ErrInvalidParameter, search)
		}

		switch len(searchInByte) {
		case ledgerstate.AddressLength:
			addr, err := findAddress(search)
			if err == nil {
				result.Address = addr
			}

		case tangle.MessageIDLength:
			messageID, err := tangle.NewMessageID(search)
			if err != nil {
				return fmt.Errorf("%w: search ID %s", ErrInvalidParameter, search)
			}

			msg, err := findMessage(messageID)
			if err == nil {
				result.Message = msg
			}

		default:
			return fmt.Errorf("%w: search ID %s", ErrInvalidParameter, search)
		}

		return c.JSON(http.StatusOK, result)
	})
}

func findMessage(messageID tangle.MessageID) (explorerMsg *ExplorerMessage, err error) {
	if !messagelayer.Tangle().Storage.Message(messageID).Consume(func(msg *tangle.Message) {
		explorerMsg = createExplorerMessage(msg)
	}) {
		err = fmt.Errorf("%w: message %s", ErrNotFound, messageID.Base58())
	}

	return
}

func findAddress(strAddress string) (*ExplorerAddress, error) {
	address, err := ledgerstate.AddressFromBase58EncodedString(strAddress)
	if err != nil {
		return nil, fmt.Errorf("%w: address %s", ErrNotFound, strAddress)
	}

	outputs := make([]ExplorerOutput, 0)

	// get outputids by address
	messagelayer.Tangle().LedgerState.OutputsOnAddress(address).Consume(func(output ledgerstate.Output) {
		var metaData *ledgerstate.OutputMetadata
		inclusionState := value.InclusionState{}
		var timestamp int64

		// get output metadata + liked status from branch of the output
		messagelayer.Tangle().LedgerState.OutputMetadata(output.ID()).Consume(func(outputMetadata *ledgerstate.OutputMetadata) {
			metaData = outputMetadata
			messagelayer.Tangle().LedgerState.BranchDAG.Branch(outputMetadata.BranchID()).Consume(func(branch ledgerstate.Branch) {
				inclusionState.Liked = branch.Liked()
			})
		})

		// get the inclusion state info from the transaction that created this output
		transactionID := output.ID().TransactionID()
		txInclusionState, _ := messagelayer.Tangle().LedgerState.TransactionInclusionState(transactionID)
		messagelayer.Tangle().LedgerState.TransactionMetadata(transactionID).Consume(func(txMeta *ledgerstate.TransactionMetadata) {
			inclusionState.Confirmed = txInclusionState == ledgerstate.Confirmed
			inclusionState.Rejected = txInclusionState == ledgerstate.Rejected
			inclusionState.Finalized = txMeta.Finalized()
			inclusionState.Conflicting = messagelayer.Tangle().LedgerState.TransactionConflicting(transactionID)
		})

		messagelayer.Tangle().LedgerState.Transaction(transactionID).Consume(func(transaction *ledgerstate.Transaction) {
			timestamp = transaction.Essence().Timestamp().Unix()
		})

		// how much pending mana the output has?
		pendingMana, _ := messagelayer.PendingManaOnOutput(output.ID())

		outputs = append(outputs, ExplorerOutput{
			ID:             jsonmodels.NewOutputID(output.ID()),
			Output:         jsonmodels.NewOutput(output),
			Metadata:       jsonmodels.NewOutputMetadata(metaData),
			InclusionState: inclusionState,
			TxTimestamp:    int(timestamp),
			PendingMana:    pendingMana,
		})
	})

	if len(outputs) == 0 {
		return nil, fmt.Errorf("%w: address %s", ErrNotFound, strAddress)
	}

	return &ExplorerAddress{
		Address:         strAddress,
		ExplorerOutputs: outputs,
	}, nil
}

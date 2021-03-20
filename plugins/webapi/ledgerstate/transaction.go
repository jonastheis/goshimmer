package ledgerstate

import (
	"net/http"

	"github.com/iotaledger/goshimmer/packages/ledgerstate"
	"github.com/iotaledger/goshimmer/packages/tangle"
	"github.com/iotaledger/goshimmer/plugins/messagelayer"
	"github.com/iotaledger/goshimmer/plugins/webapi"
	"github.com/labstack/echo"
	"github.com/mr-tron/base58"
	"golang.org/x/xerrors"
)

// region API endpoints ////////////////////////////////////////////////////////////////////////////////////////////////

// GetTransactionEndpoint is the handler for /ledgerstate/transactions/:transactionID endpoint.
func GetTransactionEndpoint(c echo.Context) (err error) {
	transactionID, err := ledgerstate.TransactionIDFromBase58(c.Param("transactionID"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, webapi.NewErrorResponse(err))
	}

	if !messagelayer.Tangle().LedgerState.Transaction(transactionID).Consume(func(transaction *ledgerstate.Transaction) {
		err = c.JSON(http.StatusOK, NewTransaction(transaction))
	}) {
		err = c.JSON(http.StatusNotFound, webapi.NewErrorResponse(xerrors.Errorf("failed to load Transaction with %s", transactionID)))
	}

	return
}

// GetTransactionMetadataEndpoint is the handler for ledgerstate/transactions/:transactionID/metadata
func GetTransactionMetadataEndpoint(c echo.Context) (err error) {
	transactionID, err := ledgerstate.TransactionIDFromBase58(c.Param("transactionID"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, webapi.NewErrorResponse(err))
	}

	if !messagelayer.Tangle().LedgerState.TransactionMetadata(transactionID).Consume(func(transactionMetadata *ledgerstate.TransactionMetadata) {
		err = c.JSON(http.StatusOK, NewTransactionMetadata(transactionMetadata))
	}) {
		return c.JSON(http.StatusNotFound, webapi.NewErrorResponse(xerrors.Errorf("failed to load TransactionMetadata of Transaction with %s", transactionID)))
	}

	return
}

// GetTransactionAttachmentsEndpoint is the handler for ledgerstate/transactions/:transactionID/attachments endpoint.
func GetTransactionAttachmentsEndpoint(c echo.Context) (err error) {
	transactionID, err := ledgerstate.TransactionIDFromBase58(c.Param("transactionID"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, webapi.NewErrorResponse(err))
	}

	var messageIDs tangle.MessageIDs
	if !messagelayer.Tangle().Storage.Attachments(transactionID).Consume(func(attachment *tangle.Attachment) {
		messageIDs = append(messageIDs, attachment.MessageID())
	}) {
		return c.JSON(http.StatusNotFound, webapi.NewErrorResponse(xerrors.Errorf("failed to load Attachments of Transaction with %s", transactionID)))
	}

	return c.JSON(http.StatusOK, NewAttachments(transactionID, messageIDs))
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Transaction ////////////////////////////////////////////////////////////////////////////////////////////////

// Transaction represents the JSON model of a ledgerstate.Transaction.
type Transaction struct {
	Version           ledgerstate.TransactionEssenceVersion `json:"version"`
	Timestamp         int64                                 `json:"timestamp"`
	AccessPledgeID    string                                `json:"accessPledgeID"`
	ConsensusPledgeID string                                `json:"consensusPledgeID"`
	Inputs            []*Input                              `json:"inputs"`
	Outputs           []*Output                             `json:"outputs"`
	UnlockBlocks      []*UnlockBlock                        `json:"unlockBlocks"`
	DataPayload       []byte                                `json:"dataPayload"`
}

// NewTransaction returns a Transaction from the given ledgerstate.Transaction.
func NewTransaction(transaction *ledgerstate.Transaction) *Transaction {
	// process inputs
	inputs := make([]*Input, len(transaction.Essence().Inputs()))
	for i, input := range transaction.Essence().Inputs() {
		inputs[i] = NewInput(input)
	}

	// process outputs
	outputs := make([]*Output, len(transaction.Essence().Outputs()))
	for i, output := range transaction.Essence().Outputs() {
		outputs[i] = NewOutput(output)
	}

	// process unlock blocks
	unlockBlocks := make([]*UnlockBlock, len(transaction.UnlockBlocks()))
	for i, unlockBlock := range transaction.UnlockBlocks() {
		ub := &UnlockBlock{
			Type:  unlockBlock.Type().String(),
			Index: i,
		}
		switch unlockBlock.Type() {
		case ledgerstate.SignatureUnlockBlockType:
			signature, _, _ := ledgerstate.SignatureFromBytes(unlockBlock.Bytes())
			ub.SignatureType = signature.Type()
			switch signature.Type() {
			case ledgerstate.ED25519SignatureType:
				signature, _, _ := ledgerstate.ED25519SignatureFromBytes(signature.Bytes())
				ub.PublicKey = signature.PublicKey.String()
				ub.Signature = signature.Signature.String()

			case ledgerstate.BLSSignatureType:
				signature, _, _ := ledgerstate.BLSSignatureFromBytes(signature.Bytes())
				ub.Signature = signature.Signature.String()
			}
		case ledgerstate.ReferenceUnlockBlockType:
			referenceUnlockBlock, _, _ := ledgerstate.ReferenceUnlockBlockFromBytes(unlockBlock.Bytes())
			ub.ReferencedIndex = referenceUnlockBlock.ReferencedIndex()
		}

		unlockBlocks[i] = ub
	}

	dataPayload := make([]byte, 0)
	if transaction.Essence().Payload() != nil {
		dataPayload = transaction.Essence().Payload().Bytes()
	}

	return &Transaction{
		Version:           transaction.Essence().Version(),
		Timestamp:         transaction.Essence().Timestamp().Unix(),
		AccessPledgeID:    base58.Encode(transaction.Essence().AccessPledgeID().Bytes()),
		ConsensusPledgeID: base58.Encode(transaction.Essence().ConsensusPledgeID().Bytes()),
		Inputs:            inputs,
		Outputs:           outputs,
		UnlockBlocks:      unlockBlocks,
		DataPayload:       dataPayload,
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Input ////////////////////////////////////////////////////////////////////////////////////////////////////////

// Input defines the Input model.
type Input struct {
	Type               string    `json:"type"`
	ReferencedOutputID *OutputID `json:"referencedOutputID"`
}

// NewInput returns an Input from the given ledgerstate.Input.
func NewInput(input ledgerstate.Input) *Input {
	if input.Type() == ledgerstate.UTXOInputType {
		return &Input{
			Type:               input.Type().String(),
			ReferencedOutputID: NewOutputID(input.(*ledgerstate.UTXOInput).ReferencedOutputID()),
		}
	}

	return nil
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region UnlockBlock //////////////////////////////////////////////////////////////////////////////////////////////////

// UnlockBlock defines the struct of a signature.
type UnlockBlock struct {
	Index           int                       `json:"index,omitempty"`
	Type            string                    `json:"type"`
	ReferencedIndex uint16                    `json:"referencedIndex,omitempty"`
	SignatureType   ledgerstate.SignatureType `json:"signatureType,omitempty"`
	PublicKey       string                    `json:"publicKey,omitempty"`
	Signature       string                    `json:"signature,omitempty"`
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region TransactionMetadata ///////////////////////////////////////////////////////////////////////////////////////////

// TransactionMetadata holds the information of a transaction metadata.
type TransactionMetadata struct {
	BranchID           string `json:"branchID"`
	Solid              bool   `json:"solid"`
	SolidificationTime int64  `json:"solidificationTime"`
	Finalized          bool   `json:"finalized"`
	LazyBooked         bool   `json:"lazyBooked"`
}

// NewTransactionMetadata returns a TransactionMetadata from the given ledgerstate.TransactionMetadata.
func NewTransactionMetadata(transactionMetadata *ledgerstate.TransactionMetadata) *TransactionMetadata {
	return &TransactionMetadata{
		BranchID:           transactionMetadata.BranchID().Base58(),
		Solid:              transactionMetadata.Solid(),
		SolidificationTime: transactionMetadata.SolidificationTime().Unix(),
		Finalized:          transactionMetadata.Finalized(),
		LazyBooked:         transactionMetadata.LazyBooked(),
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Attachments ////////////////////////////////////////////////////////////////////////////////////////////////

// Attachments is the JSON model for the attachments of a transaction.
type Attachments struct {
	TransactionID string   `json:"transactionID"`
	MessageIDs    []string `json:"messageIDs"`
}

// NewAttachments creates a new `Attachments`.
func NewAttachments(transactionID ledgerstate.TransactionID, messageIDs tangle.MessageIDs) *Attachments {
	var messageIDsBase58 []string
	for _, messageID := range messageIDs {
		messageIDsBase58 = append(messageIDsBase58, messageID.String())
	}

	return &Attachments{
		TransactionID: transactionID.Base58(),
		MessageIDs:    messageIDsBase58,
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

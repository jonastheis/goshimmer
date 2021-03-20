package message

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/iotaledger/goshimmer/packages/tangle"
	"github.com/iotaledger/goshimmer/plugins/messagelayer"
	"github.com/iotaledger/goshimmer/plugins/syncbeacon/payload"
	"github.com/iotaledger/goshimmer/plugins/webapi"
	"github.com/iotaledger/goshimmer/plugins/webapi/jsonmodels"
	"github.com/iotaledger/hive.go/node"
	"github.com/labstack/echo"
)

// region Plugin ///////////////////////////////////////////////////////////////////////////////////////////////////////

var (
	// plugin holds the singleton instance of the plugin.
	plugin *node.Plugin

	// pluginOnce is used to ensure that the plugin is a singleton.
	once sync.Once
)

// Plugin returns the plugin as a singleton.
func Plugin() *node.Plugin {
	once.Do(func() {
		plugin = node.NewPlugin("WebAPI message Endpoint", node.Enabled, func(*node.Plugin) {
			webapi.Server().GET("messages/:messageID", GetMessage)
			webapi.Server().GET("messages/:messageID/metadata", GetMessageMetadata)
			webapi.Server().POST("messages/payload", PostPayload)
		})
	})

	return plugin
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region GetMessage ///////////////////////////////////////////////////////////////////////////////////////////////////

// GetMessage is the handler for the /messages/:messageID endpoint.
func GetMessage(c echo.Context) (err error) {
	messageID, err := messageIDFromContext(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, webapi.NewErrorResponse(err))
	}

	if messagelayer.Tangle().Storage.Message(messageID).Consume(func(message *tangle.Message) {
		err = c.JSON(http.StatusOK, jsonmodels.NewMessage(message))
	}) {
		return
	}

	return c.JSON(http.StatusNotFound, webapi.NewErrorResponse(fmt.Errorf("failed to load Message with %s", messageID)))
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region GetMessageMetadata ///////////////////////////////////////////////////////////////////////////////////////////

// GetMessageMetadata is the handler for the /messages/:messageID/metadata endpoint.
func GetMessageMetadata(c echo.Context) (err error) {
	messageID, err := messageIDFromContext(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, webapi.NewErrorResponse(err))
	}

	if messagelayer.Tangle().Storage.MessageMetadata(messageID).Consume(func(messageMetadata *tangle.MessageMetadata) {
		err = c.JSON(http.StatusOK, jsonmodels.NewMessageMetadata(messageMetadata))
	}) {
		return
	}

	return c.JSON(http.StatusNotFound, webapi.NewErrorResponse(fmt.Errorf("failed to load MessageMetadata with %s", messageID)))
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region PostPayload //////////////////////////////////////////////////////////////////////////////////////////////////

// PostPayload is the handler for the /messages/payload endpoint.
func PostPayload(c echo.Context) error {
	var request PostPayloadRequest
	if err := c.Bind(&request); err != nil {
		Plugin().LogInfo(err.Error())
		return c.JSON(http.StatusBadRequest, PostPayloadResponse{Error: err.Error()})
	}

	parsedPayload, _, err := payload.FromBytes(request.Payload)
	if err != nil {
		return c.JSON(http.StatusBadRequest, PostPayloadResponse{Error: err.Error()})
	}

	msg, err := messagelayer.Tangle().IssuePayload(parsedPayload)
	if err != nil {
		return c.JSON(http.StatusBadRequest, PostPayloadResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, PostPayloadResponse{ID: msg.ID().String()})
}

// PostPayloadRequest represents the JSON model of a PostPayload request.
type PostPayloadRequest struct {
	Payload []byte `json:"payload"`
}

// PostPayloadResponse represents the JSON model of a PostPayload response.
type PostPayloadResponse struct {
	ID    string `json:"id,omitempty"`
	Error string `json:"error,omitempty"`
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region messageIDFromContext /////////////////////////////////////////////////////////////////////////////////////////

// messageIDFromContext determines the MessageID from the messageID parameter in an echo.Context. It expects it to
// either be a base58 encoded string or one of the builtin aliases (EmptyMessageID)
func messageIDFromContext(c echo.Context) (messageID tangle.MessageID, err error) {
	switch messageIDString := c.Param("messageID"); messageIDString {
	case "EmptyMessageID":
		messageID = tangle.EmptyMessageID
	default:
		messageID, err = tangle.NewMessageID(messageIDString)
	}

	return
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

package mana

import (
	"net/http"

	manaPkg "github.com/iotaledger/goshimmer/packages/mana"
	"github.com/iotaledger/goshimmer/plugins/autopeering/local"
	manaPlugin "github.com/iotaledger/goshimmer/plugins/mana"
	"github.com/iotaledger/hive.go/identity"
	"github.com/labstack/echo"
	"github.com/mr-tron/base58"
)

// getPercentile handles the request.
func getPercentile(c echo.Context) error {
	var request GetPercentileRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, GetPercentileResponse{Error: err.Error()})
	}
	ID, err := manaPkg.IDFromStr(request.Node)
	if err != nil {
		return c.JSON(http.StatusBadRequest, GetPercentileResponse{Error: err.Error()})
	}
	emptyID := identity.ID{}
	if ID == emptyID {
		ID = local.GetInstance().ID()
	}
	access, err := manaPlugin.GetManaMap(manaPkg.AccessMana).GetPercentile(ID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, GetPercentileResponse{Error: err.Error()})
	}
	consensus, err := manaPlugin.GetManaMap(manaPkg.ConsensusMana).GetPercentile(ID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, GetPercentileResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, GetPercentileResponse{
		Node:      base58.Encode(ID.Bytes()),
		Access:    access,
		Consensus: consensus,
	})
}

// GetPercentileRequest is the request object of mana/percentile.
type GetPercentileRequest struct {
	Node string `json:"node"`
}

// GetPercentileResponse holds info about the mana percentile(s) of a node.
type GetPercentileResponse struct {
	Error     string  `json:"error,omitempty"`
	Node      string  `json:"node"`
	Access    float64 `json:"access"`
	Consensus float64 `json:"consensus"`
}

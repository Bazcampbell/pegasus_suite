// betfair/betting.go

package betfair

import (
	"errors"
	"fmt"

	"pegasus_suite/betting"
	"pegasus_suite/betting/betfair/internal/exchange"
)

const (
	SideBack = string(exchange.SideBack)
	SideLay  = string(exchange.SideLay)
)

func (r BSPBetRequest) Provider() betting.Provider { return betting.ProviderBetfair }

func (r BSPBetRequest) Validate() error {
	if r.MarketID == "" {
		return errors.New("market_id is required")
	}
	if r.SelectionID <= 0 {
		return errors.New("selection_id must be positive")
	}
	if r.Side != SideBack && r.Side != SideLay {
		return fmt.Errorf("side must be %q or %q, got %q", SideBack, SideLay, r.Side)
	}
	if r.Liability <= 0 {
		return errors.New("liability must be positive")
	}
	if r.LimitPrice != 0 && r.LimitPrice < 1.01 {
		return errors.New("limit_price must be at least 1.01")
	}
	return nil
}

// PlaceBet places a BSP bet (limit-on-close when LimitPrice is set) and returns its bet ID.
func (bc *Client) PlaceBet(request betting.BetRequest) (string, error) {
	req, ok := request.(BSPBetRequest)
	if !ok {
		return "", betting.ErrWrongProvider
	}
	if err := req.Validate(); err != nil {
		return "", err
	}

	instruction := exchange.PlaceInstruction{
		OrderType:          exchange.OrderTypeMarketOnClose,
		SelectionID:        req.SelectionID,
		Side:               exchange.Side(req.Side),
		CustomerOrderRef:   req.OrderRef,
		MarketOnCloseOrder: &exchange.MarketOnCloseOrder{Liability: req.Liability},
	}
	if req.LimitPrice != 0 {
		instruction.OrderType = exchange.OrderTypeLimitOnClose
		instruction.MarketOnCloseOrder = nil
		instruction.LimitOnCloseOrder = &exchange.LimitOnCloseOrder{Liability: req.Liability, Price: req.LimitPrice}
	}

	report, err := bc.api.PlaceOrders(exchange.PlaceOrdersRequest{
		MarketID:            req.MarketID,
		CustomerStrategyRef: req.StrategyRef,
		Instructions:        []exchange.PlaceInstruction{instruction},
	})
	if err != nil {
		return "", err
	}
	if len(report.InstructionReports) == 0 {
		return "", errors.New("betfair returned no instruction report")
	}
	return report.InstructionReports[0].BetID, nil
}

var _ betting.BetRequest = BSPBetRequest{}

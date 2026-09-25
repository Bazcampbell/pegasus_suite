// betfair/bsp.go

package betfair

import (
	"errors"
	"fmt"

	"pegasus_suite/betting/betfair/internal/exchange"

	"pegasus_suite/betting"
)

func (r BSPBetRequest) Provider() betting.Provider { return betting.ProviderBetfair }

func (r BSPBetRequest) Validate() error {
	if r.MarketID == "" {
		return errors.New("market_id is required")
	}
	if r.SelectionID <= 0 {
		return errors.New("selection_id must be positive")
	}
	if _, err := parseSide(r.Side); err != nil {
		return err
	}

	if r.Liability <= 0 {
		return errors.New("liability must be positive")
	}
	if r.LimitPrice != 0 && r.LimitPrice < 1.01 {
		return errors.New("limit_price must be at least 1.01")
	}
	return nil
}

func (bc *Client) PlaceBSPBet(req BSPBetRequest) (BetResult, error) {
	if err := req.Validate(); err != nil {
		return BetResult{}, err
	}

	side, _ := parseSide(req.Side)

	instruction := exchange.PlaceInstruction{
		OrderType:        exchange.OrderTypeMarketOnClose,
		SelectionID:      req.SelectionID,
		Side:             side,
		Handicap:         req.Handicap,
		CustomerOrderRef: req.OrderRef,
	}

	if req.LimitPrice == 0 {
		instruction.MarketOnCloseOrder = &exchange.MarketOnCloseOrder{Liability: req.Liability}
	} else {
		instruction.OrderType = exchange.OrderTypeLimitOnClose
		instruction.LimitOnCloseOrder = &exchange.LimitOnCloseOrder{
			Liability: req.Liability,
			Price:     req.LimitPrice,
		}
	}

	// PlaceOrders covers the rejected status, returning error from erroneous 200 status resp
	report, err := bc.api.PlaceOrders(exchange.PlaceOrdersRequest{
		MarketID:            req.MarketID,
		CustomerRef:         req.CustomerRef,
		CustomerStrategyRef: req.CustomerStrategyRef,
		Instructions:        []exchange.PlaceInstruction{instruction},
	})
	if err != nil {
		return BetResult{}, err
	}
	if len(report.InstructionReports) == 0 {
		return BetResult{}, fmt.Errorf("betfair returned no instruction report")
	}

	placed := report.InstructionReports[0]
	return BetResult{
		BetID:      placed.BetID,
		Status:     string(placed.Status),
		PlacedDate: placed.PlacedDate,
	}, nil
}

var _ betting.BetRequest = BSPBetRequest{}

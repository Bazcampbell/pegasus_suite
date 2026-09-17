// betfair/bsp.go

package betfair

import (
	"errors"

	"racing_wagering/betting/betfair/internal/exchange"

	"racing_wagering/betting"
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

func (bc *Client) PlaceBSPBet(req BSPBetRequest) (BSPBetResult, error) {
	if err := req.Validate(); err != nil {
		return BSPBetResult{}, err
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

	// PlaceOrders already turns a rejected report into an *client.ExecutionError,
	// so a non-nil err covers both transport failure and a 200 carrying FAILURE.
	report, err := bc.api.PlaceOrders(exchange.PlaceOrdersRequest{
		MarketID:            req.MarketID,
		CustomerRef:         req.CustomerRef,
		CustomerStrategyRef: req.CustomerStrategyRef,
		Instructions:        []exchange.PlaceInstruction{instruction},
	})
	if err != nil {
		return BSPBetResult{}, err
	}
	if len(report.InstructionReports) == 0 {
		return BSPBetResult{}, errors.New("betfair returned no instruction report")
	}

	placed := report.InstructionReports[0]
	return BSPBetResult{
		BetID:      placed.BetID,
		Status:     string(placed.Status),
		PlacedDate: placed.PlacedDate,
	}, nil
}

var _ betting.BetRequest = BSPBetRequest{}

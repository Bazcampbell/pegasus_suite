// betfair/betting.go

package betfair

import (
	"errors"
	"fmt"

	"pegasus_suite/betting/betfair/internal/exchange"

	"pegasus_suite/betting"
)

func (r BetRequest) Provider() betting.Provider { return betting.ProviderBetfair }

func (r BetRequest) Validate() error {
	if r.MarketID == "" {
		return errors.New("market_id is required")
	}
	if r.SelectionID <= 0 {
		return errors.New("selection_id must be positive")
	}
	if _, err := parseSide(r.Side); err != nil {
		return err
	}

	if r.Price < 1.01 {
		return errors.New("price must be at least 1.01")
	}
	if r.Size <= 0 {
		return errors.New("size must be positive")
	}
	return nil
}

func (r BetRequest) Liability() float64 {
	if side, err := parseSide(r.Side); err == nil && side == exchange.SideLay {
		return r.Size * (r.Price - 1)
	}
	return r.Size
}

func parseSide(s string) (exchange.Side, error) {
	switch exchange.Side(s) {
	case exchange.SideBack:
		return exchange.SideBack, nil
	case exchange.SideLay:
		return exchange.SideLay, nil
	}
	return "", fmt.Errorf("side must be %q or %q, got %q", exchange.SideBack, exchange.SideLay, s)
}

func (bc *Client) PlaceBet(request betting.BetRequest) error {
	if bsp, ok := request.(BSPBetRequest); ok {
		_, err := bc.PlaceBSPBet(bsp)
		return err
	}

	req, ok := request.(BetRequest)
	if !ok {
		return betting.ErrWrongProvider
	}
	if err := req.Validate(); err != nil {
		return err
	}

	side, _ := parseSide(req.Side)

	limit := &exchange.LimitOrder{Size: req.Size, Price: req.Price}
	if req.PersistenceType != "" {
		limit.PersistenceType = exchange.PersistenceType(req.PersistenceType)
	}

	// The exchange reports a rejected order in the body with a 200; PlaceOrders
	// turns that into an *exchange.ExecutionError, so err covers both it and a
	// transport failure.
	_, err := bc.api.PlaceOrders(exchange.PlaceOrdersRequest{
		MarketID:            req.MarketID,
		CustomerRef:         req.CustomerRef,
		CustomerStrategyRef: req.CustomerStrategyRef,
		Instructions: []exchange.PlaceInstruction{{
			OrderType:        exchange.OrderTypeLimit,
			SelectionID:      req.SelectionID,
			Side:             side,
			Handicap:         req.Handicap,
			LimitOrder:       limit,
			CustomerOrderRef: req.OrderRef,
		}},
	})
	return err
}

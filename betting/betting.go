// betting/betting.go

package betting

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Provider string

const (
	ProviderBetmatic Provider = "betmatic"
	ProviderBetfair  Provider = "betfair"
)

var ErrUnknownProvider = errors.New("unknown provider")

func ParseProvider(s string) (Provider, error) {
	switch Provider(strings.ToLower(strings.TrimSpace(s))) {
	case ProviderBetmatic:
		return ProviderBetmatic, nil
	case ProviderBetfair:
		return ProviderBetfair, nil
	}
	return "", fmt.Errorf("%w: %q", ErrUnknownProvider, s)
}

// any client capable of placing a bet
// betfair, betmatic, tote (soon)
type Client interface {
	Provider() Provider
	Account() string

	RefreshToken() error
	StartTokenRefresh(ctx context.Context)
	Close()

	PlaceBet(req BetRequest) error
}

// BetRequest stays provider-specific by design. A Betmatic notification and a
// Betfair limit order have almost nothing in common beyond intent, and
// flattening them into one struct would make every field optional and push
// validation back onto every caller. The interface carries only what dispatch
// needs; the concrete type is what the client actually reads.
type BetRequest interface {
	Provider() Provider
}

var ErrWrongProvider = errors.New("bet request does not match the client's provider")

// BookieController is implemented by providers that expose per-bookmaker
// arming. Betmatic fans one notification across many bookmaker accounts, so
// which of them are live is a real control; Betfair is a single exchange
// account with nothing to arm. ENGINE asserts for this at the arm/disarm route
// rather than putting no-op methods on clients that have no bookies.
type BookieController interface {
	TurnOnBookies(bookIDs []int, botID string) []error
	TurnOffBookies(bookIDs []int, botID string) []error
}

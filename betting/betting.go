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
var ErrWrongProvider = errors.New("bet request does not match the client's provider")

func ParseProvider(s string) (Provider, error) {
	switch Provider(strings.ToLower(strings.TrimSpace(s))) {
	case ProviderBetmatic:
		return ProviderBetmatic, nil
	case ProviderBetfair:
		return ProviderBetfair, nil
	}
	return "", fmt.Errorf("%w: %q", ErrUnknownProvider, s)
}

// client capable of placing a bet
type Client interface {
	Provider() Provider
	Account() string

	RefreshToken() error
	StartTokenRefresh(ctx context.Context)
	Close()

	// a list of bet IDs that were returned for a single event + process
	// returns a summary of the bets placed during said event
	//GetEventBetRecap(id []string) (EventBetRecap, error)

	PlaceBet(req BetRequest) (id string, err error)
}

// interface carries what dispatch needs, concrete for client
type BetRequest interface {
	Provider() Provider
}

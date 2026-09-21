// strategy/strategy.go

package strategy

import (
	"pegasus_suite/apps/pegasus/core"
	"pegasus_suite/betting/betmatic"
)

func Covers(provider core.Provider, code betmatic.RacingCode) bool {
	switch provider {
	case core.ProviderTripleS:
		return code == betmatic.THOROUGHBRED || code == betmatic.HARNESS
	case core.ProviderTPD:
		return true
	}
	return false
}

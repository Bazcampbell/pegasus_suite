// packages/core/scope.go

package core

import (
	"strings"

	"pegasus_suite/betting/betmatic"
)

var ScopeCountries = []string{"AU", "US", "CA"}

func ScopeKey(country string, code betmatic.RacingCode) string {
	switch code {
	case betmatic.THOROUGHBRED:
		return country + "/THOROUGHBRED"
	case betmatic.HARNESS:
		return country + "/HARNESS"
	default:
		return ""
	}
}

func SplitScopeKey(key string) (country string, code betmatic.RacingCode, ok bool) {
	i := strings.IndexByte(key, '/')
	if i <= 0 {
		return "", "", false
	}

	switch key[i+1:] {
	case "THOROUGHBRED":
		return key[:i], betmatic.THOROUGHBRED, true
	case "HARNESS":
		return key[:i], betmatic.HARNESS, true
	}

	return "", "", false
}

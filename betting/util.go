// betting/util.go

package betting

import (
	"strings"
	"unicode"
)

// "Gold Coast", "gold-coast" and "GOLDCOAST" = same key
func NormaliseTrackKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

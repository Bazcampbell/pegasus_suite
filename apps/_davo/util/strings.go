// packages/util/strings.go

package util

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// levenshtein returns the edit distance (insertions, deletions, substitutions)
// between two strings using the standard two-row dynamic-programming table.
func Levenshtein(a, b string) int {
	ar := []rune(a)
	br := []rune(b)

	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)

	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			curr[j] = Min3(
				prev[j]+1,      // deletion
				curr[j-1]+1,    // insertion
				prev[j-1]+cost, // substitution
			)
		}
		prev, curr = curr, prev
	}

	return prev[len(br)]
}

// lookalikes maps characters that read as a Latin letter but are not one. A
// Cyrillic А and a Latin A are different runes, so a single swapped character
// makes an otherwise identical name score as a mismatch on edit distance. The
// digit entries cover the same trick done with leetspeak.
var lookalikes = map[rune]rune{
	// Cyrillic
	'А': 'A', 'В': 'B', 'Е': 'E', 'К': 'K', 'М': 'M', 'Н': 'H',
	'О': 'O', 'Р': 'P', 'С': 'C', 'Т': 'T', 'У': 'Y', 'Х': 'X',
	// Greek
	'Α': 'A', 'Β': 'B', 'Ε': 'E', 'Ζ': 'Z', 'Η': 'H', 'Ι': 'I',
	'Κ': 'K', 'Μ': 'M', 'Ν': 'N', 'Ο': 'O', 'Ρ': 'P', 'Τ': 'T',
	'Υ': 'Y', 'Χ': 'X',
	// Digits and symbols standing in for letters
	'0': 'O', '1': 'I', '3': 'E', '4': 'A', '5': 'S', '7': 'T', '8': 'B',
	'|': 'I', '!': 'I', '@': 'A', '$': 'S',
}

// Normalise reduces a runner name to a bare uppercase letter sequence for
// comparison. It is deliberately aggressive: the tipster mangles names on
// purpose, and OCR adds its own noise, so anything that does not carry identity
// (case, spacing, punctuation, accents, lookalike characters, doubled letters)
// is stripped from both sides before they are compared.
func Normalise(s string) string {
	// Decompose accents into base letter plus combining mark, drop the marks,
	// then recompose. "MOËT" and "MOET" have to reach the same string.
	stripped, _, err := transform.String(
		transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC),
		s,
	)
	if err != nil {
		// Transform only fails on malformed input; the original is still usable.
		stripped = s
	}

	stripped = strings.ToUpper(stripped)

	var b strings.Builder
	b.Grow(len(stripped))

	var prev rune
	for _, r := range stripped {
		if mapped, ok := lookalikes[r]; ok {
			r = mapped
		}

		// Everything that is not a plain letter is noise: spaces, apostrophes,
		// hyphens, emoji, and any stray punctuation the tipster or OCR added.
		if r < 'A' || r > 'Z' {
			continue
		}

		// Collapse runs of the same letter so GALLLOP and GALOP converge. Applied
		// to both sides, so a genuine double letter is not penalised.
		if r == prev {
			continue
		}

		b.WriteRune(r)
		prev = r
	}

	return b.String()
}

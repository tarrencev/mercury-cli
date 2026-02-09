package cligen

import (
	"strings"
	"unicode"
)

func kebabCase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(s) + 8)

	prevDash := false
	var prevCat runeCategory

	for _, r := range s {
		cat := categorize(r)
		switch cat {
		case catLower, catUpper, catDigit:
			if b.Len() > 0 && !prevDash {
				// Insert a dash on lower->upper boundaries (camelCase).
				if cat == catUpper && (prevCat == catLower || prevCat == catDigit) {
					b.WriteByte('-')
				}
			}
			if cat == catUpper {
				r = unicode.ToLower(r)
			}
			b.WriteRune(r)
			prevDash = false
		default:
			if b.Len() > 0 && !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
		prevCat = cat
	}

	out := b.String()
	out = strings.Trim(out, "-")
	out = strings.ReplaceAll(out, "--", "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return out
}

type runeCategory int

const (
	catOther runeCategory = iota
	catLower
	catUpper
	catDigit
)

func categorize(r rune) runeCategory {
	switch {
	case r >= 'a' && r <= 'z':
		return catLower
	case r >= 'A' && r <= 'Z':
		return catUpper
	case r >= '0' && r <= '9':
		return catDigit
	default:
		if unicode.IsLetter(r) {
			// Treat non-ASCII letters as "other" separators to keep flags predictable ASCII.
			return catOther
		}
		return catOther
	}
}

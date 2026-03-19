package mapdata

import (
	"fmt"
	"regexp"
	"strings"
)

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

// SlugifyStreet converts a street name to a filename-safe slug.
// e.g. "W 14 ST" → "w-14-st", "BROADWAY" → "broadway"
func SlugifyStreet(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonAlphaNum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// IntersectionSlug returns a combined slug from two street names,
// sorted alphabetically so order doesn't matter.
func IntersectionSlug(street1, street2 string) string {
	a := SlugifyStreet(street1)
	b := SlugifyStreet(street2)
	if a > b {
		a, b = b, a
	}
	return a + "_" + b
}

// IntersectionFilename returns a filename like "0001_broadway_w-14-st".
func IntersectionFilename(seq int, street1, street2 string) string {
	return fmt.Sprintf("%04d_%s", seq, IntersectionSlug(street1, street2))
}

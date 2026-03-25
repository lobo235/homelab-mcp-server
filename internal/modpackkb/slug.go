// Package modpackkb provides a persistent knowledge base for modpack deployment details.
package modpackkb

import (
	"regexp"
	"strings"
)

var (
	nonAlphaNum = regexp.MustCompile(`[^a-z0-9-]+`)
	multiHyphen = regexp.MustCompile(`-{2,}`)
)

// Slugify converts a human-readable name to a filesystem-safe slug.
// Example: "All the Mods 9 - To the Sky" -> "all-the-mods-9-to-the-sky"
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonAlphaNum.ReplaceAllString(s, "-")
	s = multiHyphen.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

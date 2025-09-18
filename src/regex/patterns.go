package regex

import (
	"regexp"
)

// Pre-compiled regex patterns for tokenization and text processing
var (
	// URLRegex matches URLs (http/https and www)
	URLRegex = regexp.MustCompile(`(https?://[^\s]+|www\.[^\s]+)`)

	// ApostropheRegex matches apostrophe and everything after it
	ApostropheRegex = regexp.MustCompile(`'.*`)

	// TokenizeRegex splits text on non-word characters (except apostrophes)
	TokenizeRegex = regexp.MustCompile(`[^\w']+`)
)

// CompileBannedPhrasePattern compiles a banned phrase pattern with case-insensitive matching
func CompileBannedPhrasePattern(phrase string) (*regexp.Regexp, error) {
	// Compile the pattern (case-insensitive)
	return regexp.Compile("(?i)" + phrase)
}

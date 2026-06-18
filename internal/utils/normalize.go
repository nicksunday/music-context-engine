package utils

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

func NormalizeSearchText(value string) (string, error) {
	normalizer := transform.Chain(
		norm.NFD,
		runes.Remove(runes.In(unicode.Mn)),
		norm.NFC,
	)

	preprocessed := strings.ToLower(strings.TrimSpace(value))
	preprocessed = strings.ReplaceAll(preprocessed, "&", " and ")

	normalized, _, err := transform.String(normalizer, preprocessed)
	if err != nil {
		return "", err
	}

	return strings.Join(strings.Fields(stripSearchPunctuation(normalized)), " "), nil
}

func stripSearchPunctuation(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))

	for _, r := range value {
		if unicode.IsPunct(r) {
			builder.WriteRune(' ')
			continue
		}
		builder.WriteRune(r)
	}

	return builder.String()
}

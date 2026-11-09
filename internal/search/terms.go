package search

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxQueryBytes = 512
	maxQueryWords = 32
	// minPrefixLen: shorter prefix terms match too many tokens to be useful.
	minPrefixLen = 3
)

// Words splits text into unique lower-case words, in order. Like the FTS5
// unicode61 tokenizer, letters, numbers (incl. "²") and combining marks are word
// characters, so NFD "résumé" stays one word and the tokenizer folds it.
func Words(text string) []string {
	isSeparator := func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.Is(unicode.M, r) }
	fields := strings.FieldsFunc(text, isSeparator)
	words := make([]string, 0, len(fields))
	seen := make(map[string]bool, len(fields))
	for _, f := range fields {
		w := strings.ToLower(f)
		if !seen[w] {
			seen[w] = true
			words = append(words, w)
		}
	}
	return words
}

// Expression builds a literal FTS5 OR-query from a user query.
func Expression(q string) (string, error) {
	if len(q) > maxQueryBytes || !utf8.ValidString(q) {
		return "", fmt.Errorf("query must be valid UTF-8 and at most %d bytes", maxQueryBytes)
	}
	words := Words(q)
	if len(words) == 0 || len(words) > maxQueryWords {
		return "", fmt.Errorf("query must contain 1 to %d words", maxQueryWords)
	}
	return OrExpression(words, false), nil
}

// OrExpression quotes each term, so no FTS5 syntax can be injected. With
// prefix, terms of at least minPrefixLen also match longer tokens, e.g.
// "refund" matches the identifier token "refundpayment".
func OrExpression(terms []string, prefix bool) string {
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
		if prefix && utf8.RuneCountInString(t) >= minPrefixLen {
			quoted[i] += "*"
		}
	}
	return strings.Join(quoted, " OR ")
}

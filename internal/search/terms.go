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
	// maxPhrases bounds the compound identifiers taken from one query or task.
	maxPhrases = 8
	// identifierJoiners glue words into one identifier or path, e.g.
	// memory_context, end-to-end, pkg.Func, internal/mcp/server.go.
	identifierJoiners = "_-.:/"
)

// Phrases returns the compound identifiers in text as space-separated word
// sequences, unique and in order. unicode61 splits "memory_context" into the
// adjacent tokens "memory" and "context", so the FTS5 phrase "memory context"
// matches exactly the chunks that contain the identifier.
func Phrases(text string) []string {
	isBoundary := func(r rune) bool { return isSeparator(r) && !strings.ContainsRune(identifierJoiners, r) }
	var phrases []string
	seen := map[string]bool{}
	for _, field := range strings.FieldsFunc(text, isBoundary) {
		words := Words(field)
		if len(words) < 2 {
			continue
		}
		p := strings.Join(words, " ")
		if seen[p] {
			continue
		}
		seen[p] = true
		phrases = append(phrases, p)
		if len(phrases) == maxPhrases {
			break
		}
	}
	return phrases
}

// PhraseExpression is the literal FTS5 OR-query for phrases, or "" if none.
func PhraseExpression(phrases []string) string {
	if len(phrases) == 0 {
		return ""
	}
	return OrExpression(phrases, false)
}

// isSeparator reports runes the FTS5 unicode61 tokenizer does not keep in words.
func isSeparator(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.Is(unicode.M, r)
}

// Words splits text into unique lower-case words, in order. Like the FTS5
// unicode61 tokenizer, letters, numbers (incl. "²") and combining marks are word
// characters, so NFD "résumé" stays one word and the tokenizer folds it.
func Words(text string) []string {
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

// Expression builds a literal FTS5 OR-query from a user query: its words plus
// its compound identifiers as phrases (see WithPhrases).
func Expression(q string) (string, error) {
	if len(q) > maxQueryBytes || !utf8.ValidString(q) {
		return "", fmt.Errorf("query must be valid UTF-8 and at most %d bytes", maxQueryBytes)
	}
	words := Words(q)
	if len(words) == 0 || len(words) > maxQueryWords {
		return "", fmt.Errorf("query must contain 1 to %d words", maxQueryWords)
	}
	return WithPhrases(OrExpression(words, false), Phrases(q)), nil
}

// WithPhrases ORs phrases into expr, so BM25 also credits the rarer adjacent
// sequence ("memory context") on top of its common parts.
func WithPhrases(expr string, phrases []string) string {
	if len(phrases) == 0 {
		return expr
	}
	return expr + " OR " + PhraseExpression(phrases)
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

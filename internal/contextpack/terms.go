package contextpack

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/kudaibergenoff/codememory/internal/search"
)

const (
	maxTaskBytes = 4000
	maxTerms     = 16
	minTermRunes = 2
	maxTermRunes = 40
)

// stopwords are task phrasing words that carry no code signal (English and
// Russian). They are dropped unless the task consists only of them.
var stopwords = toSet(
	"a", "an", "the", "to", "for", "of", "in", "on", "at", "by", "with", "and", "or",
	"is", "are", "be", "this", "that", "it", "as", "from", "into", "add", "adds", "create",
	"make", "implement", "fix", "update", "change", "new", "use", "using", "please",
	"should", "need", "needs", "want", "how", "what", "where", "why", "when", "which",
	"do", "does", "can", "could", "would", "we", "i", "my", "our", "some", "all",
	"between", "after", "before", "during", "about", "if", "then", "so", "but", "not",
	"no", "only", "also", "there", "they", "them", "you", "your", "its", "was", "were",
	"been", "has", "have", "had", "will", "just", "than", "too", "very", "any", "each",
	"и", "в", "во", "на", "с", "со", "по", "для", "к", "ко", "о", "об", "от", "до", "из",
	"у", "за", "не", "что", "как", "это", "нужно", "надо", "добавь", "добавить",
	"сделай", "сделать", "исправь", "исправить", "измени", "изменить", "реализуй",
	"реализовать", "новый", "новую", "новое", "пожалуйста", "чтобы", "где",
)

func toSet(words ...string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set
}

// Terms returns the search terms memory_context derives from a task.
func Terms(task string) ([]string, error) { return taskTerms(task) }

// taskTerms extracts up to maxTerms search terms from a natural-language task.
func taskTerms(task string) ([]string, error) {
	if len(task) > maxTaskBytes || !utf8.ValidString(task) {
		return nil, fmt.Errorf("task must be valid UTF-8 and at most %d bytes", maxTaskBytes)
	}
	var usable, meaningful []string
	for _, w := range search.Words(task) {
		n := utf8.RuneCountInString(w)
		if n < minTermRunes || n > maxTermRunes {
			continue
		}
		usable = append(usable, w)
		if !stopwords[w] {
			meaningful = append(meaningful, w)
		}
	}
	terms := meaningful
	if len(terms) == 0 {
		terms = usable
	}
	if len(terms) == 0 {
		return nil, errors.New("task contains no searchable words")
	}
	if len(terms) > maxTerms {
		terms = terms[:maxTerms]
	}
	return terms, nil
}

package search

import (
	"strings"
	"testing"
)

func TestLiteralQuery(t *testing.T) {
	for _, q := range []string{"", "\"*()", "\xff", strings.Repeat("x", 513)} {
		if _, err := Expression(q); err == nil {
			t.Fatalf("accepted %q", q)
		}
	}
	got, err := Expression(`Payment OR "x"; DROP TABLE files; --`)
	if err != nil || strings.Contains(got, ";") {
		t.Fatalf("unsafe %q %v", got, err)
	}
}

func TestWordsMatchFTSTokenizerBoundaries(t *testing.T) {
	cases := map[string][]string{
		"résumé file":      {"résumé", "file"}, // NFD combining marks stay inside the word
		"x² refund_payment":  {"x²", "refund", "payment"},
		"RefundPayment, v2!": {"refundpayment", "v2"},
	}
	for in, want := range cases {
		if got := Words(in); strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestPhrasesKeepCompoundIdentifiersTogether(t *testing.T) {
	cases := map[string][]string{
		"memory_context handler":           {"memory context"},
		"see internal/mcp/server.go, a-b.": {"internal mcp server go", "a b"},
		"Memory_Context memory_context":    {"memory context"},
		"RefundPayment and plain words.":   nil,
		"__init__ _x_":                     nil,
	}
	for in, want := range cases {
		if got := Phrases(in); strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestPhrasesAreCapped(t *testing.T) {
	got := Phrases(strings.Repeat("a_b c_d e_f g_h i_j k_l m_n o_p q_r ", 2))

	if len(got) != maxPhrases {
		t.Fatalf("got %d phrases", len(got))
	}
}

func TestPhraseExpressionIsLiteral(t *testing.T) {
	got := PhraseExpression([]string{`memory context`, `a "b`})

	if got != `"memory context" OR "a ""b"` {
		t.Fatalf("got %s", got)
	}
	if PhraseExpression(nil) != "" {
		t.Fatal("empty phrases must give an empty expression")
	}
}

func TestExpressionAddsCompoundIdentifierPhrases(t *testing.T) {
	got, err := Expression("memory_context handler")

	if err != nil || got != `"memory" OR "context" OR "handler" OR "memory context"` {
		t.Fatalf("got %s %v", got, err)
	}
	if WithPhrases(`"x"*`, nil) != `"x"*` {
		t.Fatal("no phrases must leave the expression unchanged")
	}
}

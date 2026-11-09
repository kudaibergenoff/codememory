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

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

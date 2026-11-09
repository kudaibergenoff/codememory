package sqlite

import (
	"strings"
	"testing"
)

func TestDSNIsAValidSQLiteURIForAnyPath(t *testing.T) {
	cases := map[string]string{
		"/tmp/p/.codememory/index.db":     "file:///tmp/p/.codememory/index.db?",
		"C:/Users/RUNNER~1/Temp/index.db": "file:///C:/Users/RUNNER~1/Temp/index.db?", // never file://C:/ (C: would be the authority)
		"/tmp/a b/index.db":               "file:///tmp/a%20b/index.db?",
	}
	for path, prefix := range cases {
		if got := dsn(path); !strings.HasPrefix(got, prefix) {
			t.Errorf("%s: got %s, want prefix %s", path, got, prefix)
		}
	}
}

package benchmark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoWithHead(t *testing.T, head string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, ".git", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("HEAD", head)
	for rel, body := range files {
		write(rel, body)
	}
	return root
}

func TestGitHeadResolvesOnlyValidRefs(t *testing.T) {
	sha := strings.Repeat("ab", 20)
	cases := map[string]struct {
		head  string
		files map[string]string
		want  string
	}{
		"loose ref":       {"ref: refs/heads/main\n", map[string]string{"refs/heads/main": sha + "\n"}, sha},
		"packed ref":      {"ref: refs/heads/main\n", map[string]string{"packed-refs": "# pack\n" + sha + " refs/heads/main\n"}, sha},
		"detached":        {sha + "\n", nil, sha},
		"traversal":       {"ref: refs/../../secret\n", map[string]string{"../secret": "TOP SECRET"}, ""},
		"outside refs":    {"ref: config\n", map[string]string{"config": "[core]"}, ""},
		"not a sha":       {"ref: refs/heads/main\n", map[string]string{"refs/heads/main": "hello world\n"}, ""},
		"garbage head":    {"PRIVATE KEY\n", nil, ""},
		"bad packed line": {"ref: refs/heads/x\n", map[string]string{"packed-refs": "notasha refs/heads/x\n"}, ""},
	}
	for name, c := range cases {
		if got := gitHead(repoWithHead(t, c.head, c.files)); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

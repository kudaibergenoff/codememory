package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kudaibergenoff/codememory/internal/config"
)

func put(t *testing.T, root, name, body string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestExclusions(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".env", ".env.example", ".aws/config", "credentials.json", "private.pem", "node_modules/a.js", ".git/config", "build/main.go"} {
		put(t, root, name, "must never index")
	}
	put(t, root, ".gitignore", "*.log\n!keep.log\n/ignored/\n**/generated/*.go\n")
	put(t, root, ".codememoryignore", "keep.log\n")
	put(t, root, "a.log", "ignored")
	put(t, root, "keep.log", "extra deny wins")
	put(t, root, "ignored/a.go", "ignored")
	put(t, root, "deep/generated/a.go", "ignored")
	put(t, root, "src/.gitignore", "*.tmp\n!keep.tmp\n")
	put(t, root, "src/no.tmp", "ignored")
	put(t, root, "src/keep.tmp", "kept")
	put(t, root, "src/main.go", "package src\nfunc Pay() {}\n")
	put(t, root, "binary.txt", "\x00\x01")
	put(t, root, "bad.txt", "\xff")
	put(t, root, "token.txt", "api_key = \"abcdefgh12345678\"")
	outside := t.TempDir()
	put(t, outside, "outside.txt", "outside")
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Log(err)
	}
	snap, err := Scan(context.Background(), root, config.Default(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, f := range snap.Files {
		found[f.Path] = true
	}
	if !found["src/main.go"] || !found["src/keep.tmp"] || snap.Secrets != 1 {
		t.Fatalf("%+v secrets=%d", found, snap.Secrets)
	}
	for _, name := range []string{"a.log", "keep.log", "src/no.tmp", "ignored/a.go", "deep/generated/a.go", "binary.txt", "bad.txt", "token.txt", "link/outside.txt", ".env"} {
		if found[name] {
			t.Errorf("included %s", name)
		}
	}
}
func TestLimitsCancelAndSymlinkIgnore(t *testing.T) {
	root := t.TempDir()
	put(t, root, "a.txt", "hello")
	put(t, root, "b.txt", "world")
	cfg := config.Default()
	cfg.MaxFiles = 1
	if _, err := Scan(context.Background(), root, cfg, nil); err == nil {
		t.Fatal("limit ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, root, config.Default(), nil); err == nil {
		t.Fatal("cancel ignored")
	}
	outside := filepath.Join(t.TempDir(), "ignore")
	if err := os.WriteFile(outside, []byte("*"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".gitignore")); err != nil {
		t.Skip(err)
	}
	if _, err := Scan(context.Background(), root, config.Default(), nil); err == nil {
		t.Fatal("symlink ignore accepted")
	}
}
func TestIgnorePatterns(t *testing.T) {
	cases := []struct {
		pattern, name string
		dir, want     bool
	}{{"/a.go", "x/a.go", false, false}, {"/a.go", "a.go", false, true}, {"a/**/b?.[ch]", "a/x/y/b1.c", false, true}, {"a/**/b.go", "a/b.go", false, true}, {"cache/", "cache", false, false}, {"cache/", "cache", true, true}, {"\\#file", "#file", false, true}, {"\\!file", "!file", false, true}, {"*.go\n!keep.go", "keep.go", false, false}}
	for _, c := range cases {
		rules, err := parseRules("", c.pattern)
		if err != nil {
			t.Fatal(err)
		}
		if got := ignored(rules, c.name, c.dir); got != c.want {
			t.Errorf("%q %q: %v", c.pattern, c.name, got)
		}
	}
	for _, c := range []struct {
		pattern, name string
		dir, want     bool
	}{{"a/**", "a", true, false}, {"a/**", "a/x", false, true}, {"[!a].go", "b.go", false, true}, {"[!a].go", "a.go", false, false}} {
		rules, err := parseRules("", c.pattern)
		if err != nil {
			t.Fatal(err)
		}
		if got := ignored(rules, c.name, c.dir); got != c.want {
			t.Errorf("%q %q = %v", c.pattern, c.name, got)
		}
	}
}

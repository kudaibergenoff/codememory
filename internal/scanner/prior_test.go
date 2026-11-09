package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kudaibergenoff/codememory/internal/config"
)

// settle backdates a file so its mtime falls outside the racy window.
func settle(t *testing.T, root, name string) time.Time {
	t.Helper()
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(filepath.Join(root, name), old, old); err != nil {
		t.Fatal(err)
	}
	return old
}

func scanWith(t *testing.T, root string, prior Prior) Snapshot {
	t.Helper()
	snap, err := Scan(context.Background(), root, config.Default(), prior)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func priorOf(snap Snapshot) Prior {
	prior := Prior{}
	for _, f := range snap.Files {
		prior[f.Path] = Entry{Hash: f.Hash, Size: f.Size, MtimeNS: f.MtimeNS}
	}
	for _, s := range snap.Skips {
		prior[s.Path] = Entry{Size: s.Size, MtimeNS: s.MtimeNS, Skip: s.Reason}
	}
	return prior
}

func fileNamed(t *testing.T, snap Snapshot, name string) File {
	t.Helper()
	for _, f := range snap.Files {
		if f.Path == name {
			return f
		}
	}
	t.Fatalf("%s not in snapshot", name)
	return File{}
}

func TestMatchingStatReusesPriorHashWithoutReading(t *testing.T) {
	root := t.TempDir()
	put(t, root, "a.go", "package a\n")
	old := settle(t, root, "a.go")
	first := fileNamed(t, scanWith(t, root, nil), "a.go")
	if first.MtimeNS != old.UnixNano() || first.Reused || first.Content == nil {
		t.Fatalf("first scan: %+v", first)
	}

	// Same size and mtime, different bytes: only a skipped read keeps the old hash.
	put(t, root, "a.go", "package b\n")
	settle(t, root, "a.go")
	second := fileNamed(t, scanWith(t, root, Prior{"a.go": {Hash: first.Hash, Size: first.Size, MtimeNS: first.MtimeNS}}), "a.go")

	if !second.Reused || second.Hash != first.Hash || second.Content != nil {
		t.Fatalf("expected stat-only reuse, got %+v", second)
	}
}

func TestChangedStatRereadsFile(t *testing.T) {
	root := t.TempDir()
	put(t, root, "a.go", "package a\n")
	settle(t, root, "a.go")
	prior := priorOf(scanWith(t, root, nil))

	put(t, root, "a.go", "package a\nfunc B() {}\n")
	settle(t, root, "a.go")
	got := fileNamed(t, scanWith(t, root, prior), "a.go")

	if got.Reused || got.Hash == prior["a.go"].Hash || got.Content == nil {
		t.Fatalf("expected re-read, got %+v", got)
	}
}

func TestRecentlyModifiedFileIsUnsettled(t *testing.T) {
	root := t.TempDir()
	put(t, root, "a.go", "package a\n")
	first := fileNamed(t, scanWith(t, root, nil), "a.go")
	if first.MtimeNS != 0 {
		t.Fatalf("recent mtime must not be trusted, got %d", first.MtimeNS)
	}

	second := fileNamed(t, scanWith(t, root, priorOf(scanWith(t, root, nil))), "a.go")

	if second.Reused {
		t.Fatal("unsettled entry matched by stat")
	}
}

func TestPriorSkipsAreReusedWithoutReading(t *testing.T) {
	root := t.TempDir()
	put(t, root, "blob.txt", "\x00\x01\x02\x03")
	put(t, root, "token.txt", `api_key = "abcdefgh12345678"`)
	settle(t, root, "blob.txt")
	settle(t, root, "token.txt")
	first := scanWith(t, root, nil)
	if len(first.Files) != 0 || len(first.Skips) != 2 || first.Secrets != 1 {
		t.Fatalf("first scan: %+v", first)
	}

	// Same size and mtime, now plain text: a skipped read keeps it excluded.
	put(t, root, "blob.txt", "text")
	settle(t, root, "blob.txt")
	second := scanWith(t, root, priorOf(first))

	if len(second.Files) != 0 || len(second.Skips) != 2 || second.Secrets != 1 {
		t.Fatalf("expected cached skips, got %+v", second)
	}
}

func TestFilesOverSizeLimitAreNotCachedAsSkips(t *testing.T) {
	root := t.TempDir()
	put(t, root, "big.txt", "0123456789")
	cfg := config.Default()
	cfg.MaxFileBytes = 4

	snap, err := Scan(context.Background(), root, cfg, nil)

	if err != nil || len(snap.Skips) != 0 || snap.Skipped != 1 {
		t.Fatalf("%+v %v", snap, err)
	}
}

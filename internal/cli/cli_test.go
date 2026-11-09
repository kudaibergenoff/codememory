package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitStatus(t *testing.T) {
	var out, stderr bytes.Buffer
	ctx := context.Background()
	root := t.TempDir()
	for _, command := range []string{"init", "init", "status", "stats"} {
		if err := Run(ctx, []string{command, "--project", root}, &out, &stderr); err != nil {
			t.Fatal(err)
		}
	}
	if err := Run(ctx, []string{"unimplemented"}, &out, &stderr); err == nil {
		t.Fatal("unknown command accepted")
	}
}

func TestIndexFullFlag(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	for _, args := range [][]string{{"init", "--project", root}, {"index", "--project", root}, {"index", "--full", "--project", root}} {
		out.Reset()
		if err := Run(context.Background(), args, &out, &errOut); err != nil {
			t.Fatalf("%v: %v %s", args, err, errOut.String())
		}
	}
	if !strings.Contains(out.String(), `"indexed":1`) {
		t.Fatalf("--full did not re-read: %s", out.String())
	}
}

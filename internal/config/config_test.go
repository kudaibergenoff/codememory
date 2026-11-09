package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAndValidate(t *testing.T) {
	root := t.TempDir()
	if err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := Init(root); err != nil {
		t.Fatal(err)
	}
	c, err := Load(root)
	if err != nil || c != Default() {
		t.Fatalf("%+v %v", c, err)
	}
	c.MaxFiles = 0
	if c.Validate() == nil {
		t.Fatal("invalid limits accepted")
	}
	if err := os.WriteFile(filepath.Join(root, ".codememory", "config.json"), []byte(`{"version":1,"unknown":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(root); err == nil {
		t.Fatal("unknown field accepted")
	}
	if err := os.WriteFile(filepath.Join(root, ".codememory", "config.json"), []byte(strings.Repeat(" ", 16385)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(root); err == nil {
		t.Fatal("oversized config accepted")
	}
}
func TestStateSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(root, ".codememory")); err != nil {
		t.Skip(err)
	}
	if Init(root) == nil {
		t.Fatal("symlink accepted")
	}
}

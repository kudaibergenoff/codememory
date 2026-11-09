package cli

import (
	"bytes"
	"context"
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

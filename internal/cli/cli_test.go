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

func TestContextCommand(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pay.go"), []byte("package pay\n\n// Refund reverses a <payment> & notifies.\nfunc Refund() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	ctx := context.Background()
	if err := Run(ctx, []string{"init", "--project", root}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	err := Run(ctx, []string{"context", "--task", "refund a payment", "--max-tokens", "600", "--project", root}, &out, &errOut)

	if err != nil {
		t.Fatalf("%v %s", err, errOut.String())
	}
	text := out.String()
	if !strings.Contains(text, `"path":"pay.go"`) || !strings.Contains(text, `"token_budget":600`) {
		t.Fatalf("unexpected output: %s", text)
	}
	if strings.Contains(text, "\\u003c") || !strings.Contains(text, "<payment> &") {
		t.Fatalf("HTML-escaped output inflates token estimates: %s", text)
	}
	if err := Run(ctx, []string{"context", "--project", root}, &out, &errOut); err == nil {
		t.Fatal("missing --task accepted")
	}
}

func TestRememberCommand(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pay.go"), []byte("package pay\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	ctx := context.Background()
	if err := Run(ctx, []string{"init", "--project", root}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	err := Run(ctx, []string{"remember", "--title", "Pay", "--body", "Payments live in pay.go", "--source", "pay.go", "--source", "pay.go", "--project", root}, &out, &errOut)

	if err != nil || !strings.Contains(out.String(), `"status":"sourced"`) {
		t.Fatalf("%v %s %s", err, out.String(), errOut.String())
	}
}

func TestStatsAndReportFilters(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pay.go"), []byte("package pay\nfunc Pay() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	ctx := context.Background()
	for _, args := range [][]string{{"init"}, {"search", "--query", "Pay"}, {"search", "--query", "Pay"}} {
		if err := Run(ctx, append(args, "--project", root), &out, &errOut); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) string {
		t.Helper()
		out.Reset()
		if err := Run(ctx, append(args, "--project", root), &out, &errOut); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return out.String()
	}

	if got := run("stats", "--last", "1d"); !strings.Contains(got, `"requests":2`) || !strings.Contains(got, `"query_cache_hit_rate":0.5`) {
		t.Fatalf("stats --last: %s", got)
	}
	if got := run("stats", "--today", "--agent", "codex"); !strings.Contains(got, `"requests":0`) {
		t.Fatalf("stats --agent codex: %s", got)
	}
	if got := run("report", "--format", "csv"); !strings.HasPrefix(got, "day,agent,channel,tool,") || strings.Count(got, "\n") != 2 {
		t.Fatalf("report csv: %q", got)
	}
	if got := run("report"); !strings.Contains(got, `"rows":[`) || !strings.Contains(got, `"verified_token_savings":null`) {
		t.Fatalf("report json: %s", got)
	}
	for _, bad := range [][]string{{"stats", "--last", "40d"}, {"stats", "--last", "abc"}, {"stats", "--today", "--last", "1d"}, {"stats", "--agent", "gpt"}, {"report", "--format", "xml"}} {
		if err := Run(ctx, append(bad, "--project", root), &out, &errOut); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

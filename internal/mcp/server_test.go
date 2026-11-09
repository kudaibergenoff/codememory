package mcp_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kudaibergenoff/codememory/internal/analytics"
	"github.com/kudaibergenoff/codememory/internal/cli"
	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/indexer"
	transport "github.com/kudaibergenoff/codememory/internal/mcp"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

func fixture(t *testing.T) (string, *sqlite.Store) {
	t.Helper()
	root := t.TempDir()
	root, err := config.Root(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pay.go"), []byte("package pay\nfunc Payment() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := sqlite.Open(context.Background(), filepath.Join(root, ".codememory"), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err = indexer.Index(context.Background(), s, root, config.Default()); err != nil {
		t.Fatal(err)
	}
	return root, s
}

func TestSDKToolsAndMetrics(t *testing.T) {
	root, s := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a, b := sdk.NewInMemoryTransports()
	ss, err := transport.New(s, root, "test").Connect(ctx, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	listed, err := session.ListTools(ctx, nil)
	if err != nil || len(listed.Tools) != 3 {
		t.Fatalf("%+v %v", listed, err)
	}
	for _, call := range []struct {
		name string
		args map[string]any
		fail bool
	}{{"memory_search", map[string]any{"query": "Payment"}, false}, {"memory_status", map[string]any{}, false}, {"memory_search", map[string]any{"query": "Payment", "limit": 1000}, true},
		{"memory_context", map[string]any{"task": "add payment refund", "max_tokens": 400}, false},
		{"memory_context", map[string]any{"task": "payment", "max_tokens": 10}, true}} {
		result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: call.name, Arguments: call.args})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError != call.fail {
			t.Fatalf("%+v", result)
		}
		text := result.Content[0].(*sdk.TextContent).Text
		if !json.Valid([]byte(text)) || strings.Contains(text, root) {
			t.Fatalf("unsafe/non-JSON response: %s", text)
		}
		if call.name != "memory_status" && !call.fail && !strings.Contains(text, "pay.go") {
			t.Fatal("missing search result")
		}
	}
	stats, err := analytics.Stats(ctx, s.DB)
	if err != nil || stats.Measured.MCPRequests != 5 || stats.Measured.Errors != 2 {
		t.Fatalf("%+v %v", stats, err)
	}
}

func TestStdioHelper(t *testing.T) {
	if os.Getenv("CODEMEMORY_TEST_HELPER") != "1" {
		return
	}
	err := cli.Run(context.Background(), []string{"serve", "--project", os.Getenv("CODEMEMORY_TEST_ROOT")}, os.Stdout, os.Stderr)
	if err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestStdioLifecycle(t *testing.T) {
	root, _ := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStdioHelper$")
	cmd.Env = append(os.Environ(), "CODEMEMORY_TEST_HELPER=1", "CODEMEMORY_TEST_ROOT="+root)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	reader := bufio.NewReader(out)
	exchange := func(request string) map[string]any {
		t.Helper()
		if _, err = io.WriteString(in, request+"\n"); err != nil {
			t.Fatal(err)
		}
		line, e := reader.ReadBytes('\n')
		if e != nil {
			t.Fatal(e)
		}
		var result map[string]any
		if e = json.Unmarshal(line, &result); e != nil {
			t.Fatalf("stdout not JSON-RPC: %q", line)
		}
		if result["jsonrpc"] != "2.0" || result["error"] != nil {
			t.Fatalf("%s", line)
		}
		return result
	}
	initialized := exchange(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	if initialized["result"].(map[string]any)["protocolVersion"] != "2025-11-25" {
		t.Fatal(initialized)
	}
	if _, err = io.WriteString(in, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n"); err != nil {
		t.Fatal(err)
	}
	exchange(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	response := exchange(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"memory_search","arguments":{"query":"Payment"}}}`)
	encoded, _ := json.Marshal(response)
	if !strings.Contains(string(encoded), "pay.go") {
		t.Fatal(string(encoded))
	}
	in.Close()
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

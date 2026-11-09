// Package scanner reads bounded text snapshots within one trusted root.
package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/kudaibergenoff/codememory/internal/config"
)

// Version changes whenever file selection or classification logic changes.
const Version = "bounded-scan-v2"

// RulesVersion also fingerprints the secret pattern, so editing it can never be
// forgotten: indexed files are otherwise not re-classified while their stat
// is unchanged.
var RulesVersion = Version + ":" + Hash([]byte(secret.String()))[:16]

// SkipReason records why a readable file was excluded after inspecting its content.
type SkipReason string

const (
	SkipContent SkipReason = "content" // binary, invalid UTF-8 or control characters
	SkipSecret  SkipReason = "secret"
)

// File is a permitted text file. Content is nil when Reused is true: the stat
// matched the prior entry, so the file was not read and Hash is the prior hash.
type File struct {
	Path, Hash, Language string
	Content              []byte
	Size                 int64
	MtimeNS              int64 // 0 means unsettled: never trusted for reuse
	Reused               bool
}

// Skipped is a file excluded by content inspection, cached to avoid re-reading.
type Skipped struct {
	Path    string
	Size    int64
	MtimeNS int64
	Reason  SkipReason
}

// Entry is the previously recorded state of one path.
type Entry struct {
	Hash    string
	Size    int64
	MtimeNS int64
	Skip    SkipReason // empty for indexed files
}

// Prior maps relative paths to their previously recorded state. A nil Prior
// forces every file to be read.
type Prior map[string]Entry

func (e Entry) matches(size, mtimeNS int64) bool {
	return e.MtimeNS != 0 && e.MtimeNS == mtimeNS && e.Size == size
}

type Snapshot struct {
	Files   []File
	Skips   []Skipped
	Skipped int
	Secrets int
}

func Hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// Scan walks root and returns permitted files. Files whose size and settled
// mtime match prior are not read (mtime is a prefilter, SHA-256 the identity).
func Scan(ctx context.Context, root string, cfg config.Config, prior Prior) (Snapshot, error) {
	if err := cfg.Validate(); err != nil {
		return Snapshot{}, err
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return Snapshot{}, errors.New("cannot open project root")
	}
	defer r.Close()
	w := &walker{ctx: ctx, root: r, cfg: cfg, prior: prior, started: time.Now()}
	if err := w.walk(".", nil, nil, 0); err != nil {
		return Snapshot{}, err
	}
	return w.snap, nil
}

func Denied(name string) bool {
	n := strings.ToLower(name)
	switch n {
	case ".git", ".codememory", ".aws", ".ssh", ".gnupg", ".azure", ".kube", ".docker", "auth.json", "secrets.json", "secrets.yaml", "secrets.yml", "node_modules", "vendor", ".venv", "venv", "dist", "build", "bin", "__pycache__", ".next", ".terraform", ".npmrc", ".pypirc", ".netrc", ".ds_store":
		return true
	}
	if strings.HasPrefix(n, ".env") || strings.Contains(n, "credential") || strings.HasPrefix(n, "id_rsa") || strings.HasPrefix(n, "id_ed25519") || strings.HasPrefix(n, "id_ecdsa") || strings.HasPrefix(n, "id_dsa") {
		return true
	}
	for _, ext := range []string{".pem", ".key", ".p12", ".pfx", ".keystore", ".sqlite", ".db"} {
		if strings.HasSuffix(n, ext) {
			return true
		}
	}
	return false
}

var secret = regexp.MustCompile(`(?im)(-----BEGIN [A-Z ]*PRIVATE KEY-----|\bAKIA[0-9A-Z]{16}\b|\bgh[pousr]_[A-Za-z0-9]{30,}|\bsk-[A-Za-z0-9_-]{24,}|(?:password|passwd|api[_-]?key|access[_-]?token|client[_-]?secret)\s*[:=]\s*["'][^"'\r\n]{8,}["'])`)

func Language(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".php":
		return "php"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".md":
		return "markdown"
	default:
		return "text"
	}
}

func read(root *os.Root, name string, limit int64) ([]byte, error) {
	// OpenRoot prevents resolution outside the root. Lstat skips stable symlinks.
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	f, err := openFile(root, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, errors.New("file replaced during open")
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("file exceeds limit")
	}
	return b, nil
}

// Package scanner reads bounded text snapshots within one trusted root.
package scanner

import (
	"bytes"
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
	"unicode"
	"unicode/utf8"

	"github.com/kudaibergenoff/codememory/internal/config"
)

type File struct {
	Path, Hash, Language string
	Content              []byte
}
type Snapshot struct {
	Files   []File
	Skipped int
	Secrets int
}

const Version = "bounded-scan-v1"

func Hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

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

func Scan(ctx context.Context, root string, cfg config.Config) (Snapshot, error) {
	var snap Snapshot
	if err := cfg.Validate(); err != nil {
		return snap, err
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return snap, errors.New("cannot open project root")
	}
	defer r.Close()
	var total int64
	var entries, patterns int
	var walk func(string, []rule, []rule, int) error
	walk = func(dir string, gitRules, extraRules []rule, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > 64 {
			return errors.New("directory depth limit exceeded")
		}
		// Copy inherited rules: sibling directories must not share appended rules.
		gitRules = append([]rule(nil), gitRules...)
		extraRules = append([]rule(nil), extraRules...)
		for i, name := range []string{".gitignore", ".codememoryignore"} {
			rel := path.Join(dir, name)
			info, e := r.Lstat(rel)
			if errors.Is(e, os.ErrNotExist) {
				continue
			}
			if e != nil {
				return errors.New("cannot inspect ignore file")
			}
			if !info.Mode().IsRegular() {
				return errors.New("ignore files must be regular, not symlinks")
			}
			b, e := read(r, rel, 64<<10)
			if e != nil {
				return errors.New("cannot read bounded ignore file")
			}
			if !utf8.Valid(b) {
				return errors.New("ignore file is not UTF-8")
			}
			base := dir
			if base == "." {
				base = ""
			}
			rules, e := parseRules(base, string(b))
			if e != nil {
				return e
			}
			patterns += len(rules)
			if patterns > 4096 {
				return errors.New("ignore rule limit exceeded")
			}
			if i == 0 {
				gitRules = append(gitRules, rules...)
			} else {
				extraRules = append(extraRules, rules...)
			}
		}
		d, e := r.Open(dir)
		if e != nil {
			return errors.New("cannot open directory")
		}
		defer d.Close()
		for {
			batch, e := d.ReadDir(128)
			if e != nil && e != io.EOF {
				return errors.New("cannot list directory")
			}
			for _, entry := range batch {
				if err := ctx.Err(); err != nil {
					return err
				}
				entries++
				if entries > 200000 {
					return errors.New("directory entry limit exceeded")
				}
				rel := path.Join(dir, entry.Name())
				if Denied(entry.Name()) || entry.Type()&os.ModeSymlink != 0 {
					snap.Skipped++
					continue
				}
				if ignored(gitRules, rel, entry.IsDir()) || ignored(extraRules, rel, entry.IsDir()) {
					snap.Skipped++
					continue
				}
				if entry.IsDir() {
					if err := walk(rel, gitRules, extraRules, depth+1); err != nil {
						return err
					}
					continue
				}
				info, e := r.Lstat(rel)
				if e != nil {
					return errors.New("file disappeared during scan; retry")
				}
				if !info.Mode().IsRegular() || info.Size() > cfg.MaxFileBytes {
					snap.Skipped++
					continue
				}
				b, e := read(r, rel, cfg.MaxFileBytes)
				if e != nil {
					return errors.New("file changed or could not be safely read; retry")
				}
				if bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b) || bytes.IndexFunc(b, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }) >= 0 {
					snap.Skipped++
					continue
				}
				if secret.Match(b) {
					snap.Skipped++
					snap.Secrets++
					continue
				}
				total += int64(len(b))
				if total > cfg.MaxTotalBytes || len(snap.Files) >= cfg.MaxFiles {
					return errors.New("project size limit exceeded; adjust config or ignore rules")
				}
				snap.Files = append(snap.Files, File{Path: rel, Hash: Hash(b), Language: Language(rel), Content: b})
			}
			if e == io.EOF {
				break
			}
		}
		return nil
	}
	err = walk(".", nil, nil, 0)
	if err != nil {
		return Snapshot{}, err
	}
	return snap, nil
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

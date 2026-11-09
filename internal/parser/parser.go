// Package parser extracts verified Go declarations and bounded line chunks.
package parser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Version is part of the index config hash: a change re-parses every file.
// v3: method symbols are named Recv.Method and functions record Refs.
const Version = "go-ast-text-v3"

// KindImport symbols record Go import paths as AST facts. They are not used as
// chunk labels, which would weight unrelated chunks by imported package names.
const KindImport = "import"

// LanguageSupport describes how deeply a detected language is understood.
type LanguageSupport struct {
	Language string `json:"language"`
	Support  string `json:"support"`
}

// Supported lists every language the scanner detects and what the index holds for it.
var Supported = []LanguageSupport{
	{"go", "go/ast symbols (package, types, funcs, methods, imports) and text chunks"},
	{"python", "text chunks"},
	{"php", "text chunks"},
	{"typescript", "text chunks"},
	{"javascript", "text chunks"},
	{"markdown", "text chunks"},
	{"text", "text chunks (other UTF-8 files)"},
}

type Symbol struct {
	Name, Kind string
	Start, End int
}
type Chunk struct {
	Symbol, Content string
	Start, End      int
}
type Result struct {
	Symbols    []Symbol
	Chunks     []Chunk
	Refs       []Ref // Go only: references from functions and methods
	ParseError bool
}

func Parse(name, language string, content []byte, chunkLines int) Result {
	var result Result
	if language == "go" {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, name, content, parser.SkipObjectResolution)
		if err != nil {
			result.ParseError = true
		} else {
			add := func(n ast.Node, name, kind string) {
				result.Symbols = append(result.Symbols, Symbol{name, kind, fset.Position(n.Pos()).Line, fset.Position(n.End()).Line})
			}
			add(f.Name, f.Name.Name, "package")
			for _, imp := range f.Imports {
				if p, err := strconv.Unquote(imp.Path.Value); err == nil {
					add(imp, p, KindImport)
				}
			}
			for _, decl := range f.Decls {
				switch n := decl.(type) {
				case *ast.FuncDecl:
					kind := "func"
					if n.Recv != nil {
						kind = "method"
					}
					add(n, funcName(n), kind)
				case *ast.GenDecl:
					for _, spec := range n.Specs {
						if s, ok := spec.(*ast.TypeSpec); ok {
							kind := "type"
							if _, ok := s.Type.(*ast.InterfaceType); ok {
								kind = "interface"
							}
							add(s, s.Name.Name, kind)
						}
					}
				}
			}
			result.Refs = fileRefs(fset, f)
		}
	}
	lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	if len(content) == 0 {
		return result
	}
	if chunkLines < 1 {
		chunkLines = 40
	}
	// Bound long lines by UTF-8 byte size as well as line count.
	for start := 0; start < len(lines); {
		end := start
		size := 0
		for end < len(lines) && end-start < chunkLines {
			next := len(lines[end]) + 1
			if end > start && size+next > 4096 {
				break
			}
			size += next
			end++
			if size >= 4096 {
				break
			}
		}
		text := strings.Join(lines[start:end], "\n")
		symbol := ""
		for _, s := range result.Symbols {
			if s.Kind != KindImport && s.Start <= end && s.End >= start+1 {
				if symbol != "" {
					symbol += " "
				}
				symbol += s.Name
				if len(symbol) > 1024 {
					cut := 1024
					for !utf8.RuneStart(symbol[cut]) {
						cut--
					}
					symbol = symbol[:cut]
					break
				}
			}
		}
		for len(text) > 4096 {
			cut := 4096
			for !utf8.RuneStart(text[cut]) {
				cut--
			}
			result.Chunks = append(result.Chunks, Chunk{symbol, text[:cut], start + 1, end})
			text = text[cut:]
		}
		if text != "" {
			result.Chunks = append(result.Chunks, Chunk{symbol, text, start + 1, end})
		}
		start = end
	}
	return result
}

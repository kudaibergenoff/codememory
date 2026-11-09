package parser

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestGoAST(t *testing.T) {
	r := Parse("main.go", "go", []byte("package pay\ntype Service struct{}\ntype Payer interface { Pay() }\nfunc (s *Service) Pay() {}\nfunc New() {}\n"), 2)
	if r.ParseError || len(r.Symbols) != 5 {
		t.Fatalf("%+v", r)
	}
	if r.Symbols[2].Kind != "interface" || r.Symbols[3].Kind != "method" {
		t.Fatalf("%+v", r.Symbols)
	}
	if len(r.Chunks) != 3 || r.Chunks[1].Start != 3 || r.Chunks[1].End != 4 {
		t.Fatalf("%+v", r.Chunks)
	}
}
func TestMalformedAndUnicode(t *testing.T) {
	r := Parse("a.go", "go", []byte("package broken {"), 40)
	if !r.ParseError || len(r.Chunks) != 1 || len(r.Symbols) != 0 {
		t.Fatalf("%+v", r)
	}
	r = Parse("a.txt", "text", []byte(strings.Repeat("界", 4000)), 40)
	var joined string
	for _, c := range r.Chunks {
		if !utf8.ValidString(c.Content) || len(c.Content) > 4096 {
			t.Fatal("bad UTF-8 chunk")
		}
		joined += c.Content
	}
	if joined != strings.Repeat("界", 4000) {
		t.Fatal("lost text")
	}
}
func BenchmarkGoAST(b *testing.B) {
	src := []byte("package pay\ntype Service struct{}\nfunc (s *Service) Pay() {}\n")
	b.ReportAllocs()
	for b.Loop() {
		Parse("a.go", "go", src, 40)
	}
}

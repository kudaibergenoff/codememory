package parser

import (
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"strconv"
	"strings"
)

// Ref kinds. RefCall and RefUse name package-level identifiers; RefMethod is a
// method called on a value whose type is unknown without type checking.
const (
	RefCall   = "call"
	RefUse    = "ref"
	RefMethod = "method"
)

// MethodPkg marks a RefMethod target: matched to methods by name only.
const MethodPkg = "."

// Ref is one distinct reference from a top-level function or method (From,
// "Recv.Method" for methods) to a name. Pkg is the import path for qualified
// names, "" for the same package and MethodPkg for method calls. Line is the
// first use; Uses counts every use in From. These are syntactic go/ast facts:
// there is no type checking, so same-package names may also match locals that
// shadow them, and method targets are ambiguous.
type Ref struct {
	From, Pkg, Name, Kind string
	Line, Uses            int
}

var majorVersion = regexp.MustCompile(`^v[0-9]+$`)

// importName guesses the package name of an import path from its last element,
// skipping major-version suffixes ("/v2", ".v3") and a "go-" prefix.
func importName(path string) string {
	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]
	if majorVersion.MatchString(name) && len(parts) > 1 {
		name = parts[len(parts)-2]
	}
	if i := strings.Index(name, ".v"); i > 0 && majorVersion.MatchString(name[i+1:]) {
		name = name[:i]
	}
	return strings.TrimPrefix(name, "go-")
}

// fileImports maps local package names to import paths. Blank and dot imports
// are left out: their identifiers cannot be attributed syntactically.
func fileImports(f *ast.File) map[string]string {
	out := map[string]string{}
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		name := importName(p)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name != "_" && name != "." {
			out[name] = p
		}
	}
	return out
}

// receiverType returns the base type name of a method receiver.
func receiverType(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	expr := recv.List[0].Type
	for {
		switch e := expr.(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		case *ast.ParenExpr:
			expr = e.X
		case *ast.Ident:
			return e.Name
		default:
			return ""
		}
	}
}

// funcName is Name for functions and Recv.Name for methods.
func funcName(fn *ast.FuncDecl) string {
	if recv := receiverType(fn.Recv); recv != "" {
		return recv + "." + fn.Name.Name
	}
	return fn.Name.Name
}

// fileRefs collects the references of every function and method in f.
func fileRefs(fset *token.FileSet, f *ast.File) []Ref {
	imports := fileImports(f)
	declared := topLevelNames(f)
	var refs []Ref
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			refs = append(refs, funcRefs(fset, fn, imports, declared)...)
		}
	}
	return refs
}

// topLevelNames lists the functions, types, variables and constants f declares,
// so a file that defines its own min or max keeps references to it.
func topLevelNames(f *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil {
				names[d.Name.Name] = true
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					names[s.Name.Name] = true
				case *ast.ValueSpec:
					for _, n := range s.Names {
						names[n.Name] = true
					}
				}
			}
		}
	}
	return names
}

// callee strips type arguments and parentheses: Gen[int](x) calls Gen.
func callee(fun ast.Expr) ast.Expr {
	for {
		switch e := fun.(type) {
		case *ast.IndexExpr:
			fun = e.X
		case *ast.IndexListExpr:
			fun = e.X
		case *ast.ParenExpr:
			fun = e.X
		default:
			return fun
		}
	}
}

// isKeyed reports composite literals whose keys are values (maps, arrays and
// slices), not struct field names.
func isKeyed(lit *ast.CompositeLit) bool {
	switch lit.Type.(type) {
	case *ast.MapType, *ast.ArrayType:
		return true
	}
	return false
}

// isBuiltin reports predeclared names the file does not redeclare.
func isBuiltin(name string, declared map[string]bool) bool {
	return !declared[name] && types.Universe.Lookup(name) != nil
}

// refCollector deduplicates references of one function by (Pkg, Name).
type refCollector struct {
	fset  *token.FileSet
	from  string
	index map[[2]string]int
	refs  []Ref
}

func (c *refCollector) add(pkg, name, kind string, pos token.Pos) {
	key := [2]string{pkg, name}
	if i, ok := c.index[key]; ok {
		c.refs[i].Uses++
		if kind == RefCall {
			c.refs[i].Kind = RefCall
		}
		return
	}
	c.index[key] = len(c.refs)
	c.refs = append(c.refs, Ref{From: c.from, Pkg: pkg, Name: name, Kind: kind, Line: c.fset.Position(pos).Line, Uses: 1})
}

func funcRefs(fset *token.FileSet, fn *ast.FuncDecl, imports map[string]string, declared map[string]bool) []Ref {
	locals := localNames(fn)
	c := &refCollector{fset: fset, from: funcName(fn), index: map[[2]string]int{}}
	skip := map[*ast.Ident]bool{}
	called := map[ast.Expr]bool{}
	visit := func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			called[callee(n.Fun)] = true
		case *ast.SelectorExpr:
			skip[n.Sel] = true
			if x, ok := n.X.(*ast.Ident); ok && !locals[x.Name] && imports[x.Name] != "" {
				skip[x] = true
				c.add(imports[x.Name], n.Sel.Name, kindOf(called[n]), n.Sel.Pos())
			} else if called[n] {
				c.add(MethodPkg, n.Sel.Name, RefMethod, n.Sel.Pos())
			}
		case *ast.CompositeLit:
			if isKeyed(n) {
				break
			}
			for _, elt := range n.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if k, ok := kv.Key.(*ast.Ident); ok {
						skip[k] = true // struct field name, also in elided {...}
					}
				}
			}
		case *ast.Field:
			for _, name := range n.Names {
				skip[name] = true
			}
		case *ast.LabeledStmt:
			skip[n.Label] = true
		case *ast.BranchStmt:
			if n.Label != nil {
				skip[n.Label] = true
			}
		case *ast.Ident:
			if !skip[n] && !locals[n.Name] && n.Name != "_" && !isBuiltin(n.Name, declared) {
				c.add("", n.Name, kindOf(called[n]), n.Pos())
			}
		}
		return true
	}
	ast.Inspect(fn.Type, visit)
	if fn.Body != nil {
		ast.Inspect(fn.Body, visit)
	}
	return c.refs
}

func kindOf(called bool) string {
	if called {
		return RefCall
	}
	return RefUse
}

// localNames lists every name a function declares: receiver, parameters,
// results, type parameters and declarations inside its body. Scopes are not
// tracked, so a local anywhere hides a package-level name everywhere in fn.
func localNames(fn *ast.FuncDecl) map[string]bool {
	names := map[string]bool{}
	addFields := func(fl *ast.FieldList) {
		if fl == nil {
			return
		}
		for _, f := range fl.List {
			for _, n := range f.Names {
				names[n.Name] = true
			}
		}
	}
	addFields(fn.Recv)
	ast.Inspect(fn, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncType:
			addFields(n.TypeParams)
			addFields(n.Params)
			addFields(n.Results)
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE {
				for _, lhs := range n.Lhs {
					if id, ok := lhs.(*ast.Ident); ok {
						names[id.Name] = true
					}
				}
			}
		case *ast.RangeStmt:
			if n.Tok == token.DEFINE {
				for _, e := range []ast.Expr{n.Key, n.Value} {
					if id, ok := e.(*ast.Ident); ok {
						names[id.Name] = true
					}
				}
			}
		case *ast.ValueSpec:
			for _, id := range n.Names {
				names[id.Name] = true
			}
		case *ast.TypeSpec:
			names[n.Name.Name] = true
		}
		return true
	})
	return names
}

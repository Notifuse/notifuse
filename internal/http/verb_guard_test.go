package http

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every handler that reads a request body must refuse the wrong verb.
//
// The mux matches on path alone, so a handler that decodes its body without checking the
// method performs its write on any verb. Five did: customEvents.upsert and .import wrote
// contact events on a GET, user.signin SENT AN EMAIL on one, user.verify minted a session,
// and user.updateLanguage wrote. Each was a plain omission — every other write in the package
// guards — and nothing could see them, because a guard is a convention rather than a type.
//
// It is a source scan, not a route drive, and that is deliberate. Driving routes needs a list
// of which ones mutate, and a list is the thing that rots: the previous attempt at this
// guarantee was a hand-maintained ledger of every route, and a handler added without a guard
// was exactly what it failed to notice. "Reads a body" is a property of the code, so the
// compiler's own parse tree can be asked for it and no list has to be kept in step.
//
// Guarded means one of three things, and the third is why this is an AST walk rather than a
// grep: the function checks the method itself, or it calls a function that does, or it is
// CALLED BY one that does — notification_center's dispatcher switches on the verb and hands
// off to a get and a post half, and neither half checks anything.
func TestEveryBodyDecodingHandlerRefusesTheWrongVerb(t *testing.T) {
	fset := token.NewFileSet()
	files := parseHandlerPackage(t, fset)

	type fn struct {
		file    string
		decl    *ast.FuncDecl
		decodes bool
		guards  bool
	}

	byName := map[string]*fn{}
	var all []*fn
	for path, file := range files {
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Body == nil {
				continue
			}
			request := requestParamName(funcDecl)
			f := &fn{
				file:    filepath.Base(path),
				decl:    funcDecl,
				decodes: request != "" && readsRequestBody(funcDecl.Body, request),
				guards:  request != "" && checksMethod(funcDecl.Body, request),
			}
			all = append(all, f)
			byName[funcDecl.Name.Name] = f
		}
	}

	// The scan is only as good as what it matched. A walker that silently found nothing
	// would report no unguarded handlers and pass having checked nothing at all — which is
	// the failure mode of every source-scanning test.
	var decoders, guardians int
	for _, f := range all {
		if f.decodes {
			decoders++
		}
		if f.guards {
			guardians++
		}
	}
	require.Greater(t, decoders, 50,
		"the walker found %d handlers reading a request body; this package has around a hundred, so it is looking in the wrong place and would never fail", decoders)
	require.Greater(t, guardians, 20,
		"the walker found %d handlers checking the method; if it cannot see a guard it will report every handler as unguarded", guardians)

	calls := func(f *fn) []string { return calledNames(f.decl.Body) }

	guardedThroughACall := func(f *fn) bool {
		for _, name := range calls(f) {
			if other, ok := byName[name]; ok && other != f && other.guards {
				return true
			}
		}
		return false
	}

	guardedByItsCaller := func(f *fn) bool {
		for _, candidate := range all {
			if candidate == f || !candidate.guards {
				continue
			}
			for _, name := range calls(candidate) {
				if name == f.decl.Name.Name {
					return true
				}
			}
		}
		return false
	}

	var unguarded []string
	for _, f := range all {
		if !f.decodes || f.guards {
			continue
		}
		if guardedThroughACall(f) || guardedByItsCaller(f) {
			continue
		}
		unguarded = append(unguarded, f.file+" "+f.decl.Name.Name)
	}
	sort.Strings(unguarded)

	assert.Empty(t, unguarded,
		"these handlers decode a request body without checking the method, so they perform their "+
			"write on any verb the mux routes to them:\n  %s\n\n"+
			"Add the guard every other write in this package has:\n"+
			"    if r.Method != http.MethodPost {\n"+
			"        WriteJSONError(w, \"Method not allowed\", http.StatusMethodNotAllowed)\n"+
			"        return\n"+
			"    }\n"+
			"A handler whose guard lives in a dispatcher or a shared helper is already accepted; "+
			"do not weaken this test to accommodate a new shape without checking that the shape "+
			"actually guards.",
		strings.Join(unguarded, "\n  "))
}

// parseHandlerPackage parses every non-test source file of this package.
func parseHandlerPackage(t *testing.T, fset *token.FileSet) map[string]*ast.File {
	t.Helper()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	files := map[string]*ast.File{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, perr := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, perr, "could not parse %s", name)
		files[name] = parsed
	}
	require.NotEmpty(t, files, "no handler sources were parsed")
	return files
}

// requestParamName returns the identifier bound to the *http.Request parameter, or "" when
// the function takes none — which is how a non-handler is recognised without a name
// convention.
func requestParamName(decl *ast.FuncDecl) string {
	if decl.Type.Params == nil {
		return ""
	}
	for _, field := range decl.Type.Params.List {
		star, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		selector, ok := star.X.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Request" {
			continue
		}
		pkg, ok := selector.X.(*ast.Ident)
		if !ok || pkg.Name != "http" {
			continue
		}
		if len(field.Names) == 0 {
			return ""
		}
		return field.Names[0].Name
	}
	return ""
}

// readsRequestBody reports whether the body mentions <request>.Body, which is every way this
// package reads a payload: json.NewDecoder(r.Body), io.ReadAll(r.Body), r.Body.Close.
func readsRequestBody(body *ast.BlockStmt, request string) bool {
	return mentionsSelector(body, request, "Body")
}

// checksMethod reports whether the body reads <request>.Method.
func checksMethod(body *ast.BlockStmt, request string) bool {
	return mentionsSelector(body, request, "Method")
}

func mentionsSelector(node ast.Node, receiver, field string) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		selector, ok := n.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != field {
			return true
		}
		ident, ok := selector.X.(*ast.Ident)
		if ok && ident.Name == receiver {
			found = true
			return false
		}
		return true
	})
	return found
}

// calledNames returns every function or method name invoked in the body, unqualified, so a
// guard reached through h.decode(...) or a bare helper is both visible.
func calledNames(body *ast.BlockStmt) []string {
	var names []string
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			names = append(names, fun.Name)
		case *ast.SelectorExpr:
			names = append(names, fun.Sel.Name)
		}
		return true
	})
	return names
}

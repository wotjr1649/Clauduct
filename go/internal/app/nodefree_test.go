package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// REL03 / requirement V2-02: the Go product must not depend on Node at runtime.
//
// This guards against a specific, already-identified way the dependency gets back in. The
// Node baseline hands the native client command strings built from its own interpreter
// path: native-protocol.mjs composes "<node.exe> <repo>/src/review-diff.mjs" for file
// review, and clauduct.mjs composes the same shape for its routing hooks. Porting either
// mechanically would make a Go binary require a Node install and a repository checkout,
// which is exactly what H01 says it must not.
//
// It scans string literals from the parsed AST rather than raw file text. A dependency has
// to be a literal somewhere — an executable name, a path, an argument — so literals are the
// whole surface. Scanning raw text instead would fail on a comment that merely names the
// baseline file it is porting from, which is how the first version of this test failed:
// prose is not a dependency and a guard that cannot tell the difference gets disabled.
func TestProductSourceHasNoNodeRuntimeDependency(t *testing.T) {
	root := moduleRoot(t)

	forbidden := []struct{ fragment, why string }{
		{"." + "mjs", "a Node module path means the binary needs a repository checkout"},
		{"node." + "exe", "a Node executable name means the binary needs a Node install"},
		{"npm" + ".cmd", "an npm shim means the binary shells out to a package manager"},
		{"Dotnet" + "HttpProbe", "the .NET verification probe is not a product dependency"},
	}

	for _, path := range productSources(t, root) {
		fset := token.NewFileSet()
		// Comments are deliberately not parsed: they cannot create a dependency.
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		rel, _ := filepath.Rel(root, path)
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				value = lit.Value
			}
			lowered := strings.ToLower(value)
			for _, f := range forbidden {
				if strings.Contains(lowered, strings.ToLower(f.fragment)) {
					t.Errorf("%s:%d string literal %q contains %q: %s",
						filepath.ToSlash(rel), fset.Position(lit.Pos()).Line, value, f.fragment, f.why)
				}
			}
			return true
		})
	}
}

// The module must stay dependency-free until a dependency is argued for. Every third-party
// package is supply chain, licence and update cost the Node baseline did not have: it had
// zero npm dependencies, so there is no existing surface to reproduce.
func TestModuleHasNoThirdPartyDependencies(t *testing.T) {
	root := moduleRoot(t)

	if _, err := os.Stat(filepath.Join(root, "go.sum")); err == nil {
		t.Error("go.sum exists; a dependency was added without a recorded decision")
	}
	content, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if strings.Contains(string(content), "require") {
		t.Errorf("go.mod has a require block:\n%s", content)
	}
}

func productSources(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no product sources found; the scan would pass vacuously")
	}
	return paths
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test working directory")
		}
		dir = parent
	}
}

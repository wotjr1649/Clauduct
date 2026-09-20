package app

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
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
					// These two paths name the embedded module loaded by native's
					// own runtime. They are not Node programs or checkout paths.
					// The companion test verifies embedded bytes, runtime-free source
					// and the exact closed exception; every other module path fails.
					if f.fragment == ".mjs" && embeddedNativeModulePath(filepath.ToSlash(rel), value) {
						continue
					}
					t.Errorf("%s:%d string literal %q contains %q: %s",
						filepath.ToSlash(rel), fset.Position(lit.Pos()).Line, value, f.fragment, f.why)
				}
			}
			return true
		})
	}
}

func embeddedNativeModulePath(file, value string) bool {
	return file == "internal/app/native_events.go" && (value == `{"modules":["./events.mjs"]}` || value == "hooks/events.mjs")
}

func TestEmbeddedNativeModuleDoesNotIntroduceExternalRuntime(t *testing.T) {
	source, err := os.ReadFile("native-events.mjs")
	if err != nil || string(source) != nativeEventModule {
		t.Fatal("native module not embedded byte-for-byte")
	}
	for _, forbidden := range []string{"import ", "import(", "require(", "node:", "process.", "eval(", "new Function", "globalThis", "fetch("} {
		if strings.Contains(nativeEventModule, forbidden) {
			t.Fatalf("unreviewed runtime capability: %s", forbidden)
		}
	}
	for _, tc := range []struct {
		file, value string
		want        bool
	}{
		{"internal/app/native_events.go", "hooks/events.mjs", true},
		{"internal/app/native_events.go", `{"modules":["./events.mjs"]}`, true},
		{"internal/app/run.go", "hooks/events.mjs", false},
		{"internal/app/native_events.go", "node.exe hooks/events.mjs", false},
		{"internal/app/native_events.go", "hooks/other.mjs", false},
		{"internal/app/native_events.go", `{"modules":["../../checkout/events.mjs"]}`, false},
	} {
		if embeddedNativeModulePath(tc.file, tc.value) != tc.want {
			t.Fatal("runtime exception widened")
		}
	}
}

// The dependency decision and boundary review are recorded in
// verification/policy-evidence-20260918/DEPENDENCIES.md. Keep the set closed:
// exact tokenization and RFC6455 framing are not reimplemented in application code.
func TestModuleUsesOnlyReviewedPinnedDependencies(t *testing.T) {
	root := moduleRoot(t)
	cmd := exec.Command("go", "mod", "edit", "-json")
	cmd.Dir = root
	raw, err := cmd.Output()
	if err != nil {
		t.Fatal("read module graph", err)
	}
	var module struct {
		Require          []struct{ Path, Version string }
		Replace, Exclude []json.RawMessage
	}
	if json.Unmarshal(raw, &module) != nil {
		t.Fatal("module graph invalid")
	}
	allowed := map[string]string{"github.com/tiktoken-go/tokenizer": "v0.8.1", "github.com/dlclark/regexp2/v2": "v2.5.1", "github.com/coder/websocket": "v1.8.15", "go.yaml.in/yaml/v3": "v3.0.4"}
	if len(module.Replace) != 0 || len(module.Exclude) != 0 || len(module.Require) != len(allowed) {
		t.Fatal("dependency set differs from reviewed decision")
	}
	for _, dependency := range module.Require {
		if allowed[dependency.Path] != dependency.Version {
			t.Errorf("unreviewed dependency %s %s", dependency.Path, dependency.Version)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "go.sum")); err != nil {
		t.Fatal("pinned dependency checksums missing")
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

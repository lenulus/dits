// Package pilot hosts only the forbidden-imports lint test (§8.3, §8.4
// TestForbiddenImports). It has no runtime code: Pilot's actual code lives
// in subpackages (mcp, auth, signing, scheduler, projections, store,
// web/...). A package consisting solely of a _test.go file is valid Go and
// lets `go test ./internal/pilot/...` pick this up.
package pilot

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// modulePath is the Go module path for this repo (see go.mod).
const modulePath = "github.com/lenulus/pf"

// allowedPrefix is the only internal import prefix Pilot code may use.
// Anything under internal/pilot (including internal/pilot itself) is fine.
const allowedPrefix = modulePath + "/internal/pilot"

// internalPrefix matches any DITS-internal package import.
const internalPrefix = modulePath + "/internal/"

// TestForbiddenImports statically scans every non-test-excluded .go file
// under cmd/pilot/ and internal/pilot/ and fails if any imports a
// github.com/lenulus/pf/internal/<x> package that is not internal/pilot or
// a subpackage of it. This enforces the §2 topology: Pilot reaches DITS
// only over MCP, never by importing internal/domain, internal/workops,
// internal/mcp, internal/server, internal/store, internal/project,
// internal/sync, internal/crypto, internal/blob, etc.
func TestForbiddenImports(t *testing.T) {
	root := moduleRoot(t)

	roots := []string{
		filepath.Join(root, "cmd", "pilot"),
		filepath.Join(root, "internal", "pilot"),
	}

	fset := token.NewFileSet()
	var scanned int

	for _, dir := range roots {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			// Skip this lint test itself so it can name forbidden paths in
			// comments/strings without tripping the scan. (We parse imports
			// only, so comments wouldn't match anyway, but be explicit.)
			if filepath.Base(path) == "forbidden_imports_test.go" {
				return nil
			}

			f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if perr != nil {
				t.Fatalf("parse %s: %v", path, perr)
			}
			scanned++

			for _, spec := range f.Imports {
				imp, uerr := strconv.Unquote(spec.Path.Value)
				if uerr != nil {
					t.Fatalf("unquote import in %s: %v", path, uerr)
				}
				if !strings.HasPrefix(imp, internalPrefix) {
					continue // external or non-internal import — allowed
				}
				// It's a DITS-internal import; allow only internal/pilot[/...].
				if imp == allowedPrefix || strings.HasPrefix(imp, allowedPrefix+"/") {
					continue
				}
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s imports forbidden DITS-internal package %q; Pilot must use MCP only (plan §2, §8.1)", rel, imp)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}

	if scanned == 0 {
		t.Fatalf("scanned no .go files under %v — roots wrong?", roots)
	}
}

// moduleRoot walks up from this test file's directory until it finds the
// directory containing go.mod, returning that path. This keeps the test
// self-contained (no x/tools dependency) and independent of the working
// directory.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed; cannot locate module root")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("reached filesystem root without finding go.mod")
		}
		dir = parent
	}
}

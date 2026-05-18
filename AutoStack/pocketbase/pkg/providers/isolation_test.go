package providers

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Phase 3.9.4 — Provider isolation enforcement (HC-6).
//
// AutoStack's provider isolation rule (HC-6): provider-specific cloud SDKs
// MUST be confined to that provider's package directory. The reconciler
// and provider-agnostic code MUST NOT import aws-sdk, google-cloud, or
// azure-sdk packages directly — only via the Provider interface.
//
// This test FAILS the build if any forbidden import is found outside its
// allowed package. Previously this rule was discipline-enforced (HC-6
// review checklist). Phase 3.9.4 makes it machine-enforced.
//
// Why a test instead of a vet pass? `go test` runs in CI; it covers all
// platforms; no separate tool needs installation. A custom linter would
// fragment the build pipeline.

// forbiddenImports maps an import path prefix to the directory prefixes
// allowed to use it. If a Go file outside the allowed prefixes imports
// from a forbidden prefix, the test fails.
var forbiddenImports = map[string][]string{
	"github.com/aws/aws-sdk-go-v2": {
		"pkg/providers/ecs/",
	},
	"github.com/aws/aws-sdk-go":  {"pkg/providers/ecs/"},
	"cloud.google.com/go":        {"pkg/providers/cloudrun/", "pkg/secrets/"},
	"google.golang.org/api":      {"pkg/providers/cloudrun/", "pkg/secrets/"},
	"github.com/Azure":           {"pkg/providers/aca/"},
}

// TestProviderIsolation walks the source tree under the repo root and
// checks every .go file's imports against the forbiddenImports map.
// Fails immediately on the first violation with a precise error message.
func TestProviderIsolation(t *testing.T) {
	// Walk from the module root (two levels up from pkg/providers).
	root, err := findModuleRoot()
	if err != nil {
		t.Skipf("cannot locate module root (skipping in non-checkout): %v", err)
	}

	violations := 0
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable paths
		}
		if info.IsDir() {
			// Skip vendored/external trees.
			name := info.Name()
			if name == "vendor" || name == "node_modules" || name == ".git" ||
				name == "pb_data" || name == "pb_migrations" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Skip the isolation test itself.
		if strings.HasSuffix(path, "isolation_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		// Normalize to forward slashes for cross-platform stability.
		rel = filepath.ToSlash(rel)

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Logf("skip unparsable file %s: %v", rel, err)
			return nil
		}
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for forbidden, allowedPrefixes := range forbiddenImports {
				if !strings.HasPrefix(path, forbidden) {
					continue
				}
				allowed := false
				for _, prefix := range allowedPrefixes {
					if strings.Contains(rel, prefix) {
						allowed = true
						break
					}
				}
				if !allowed {
					t.Errorf("HC-6 ISOLATION VIOLATION: %s imports %q (allowed only under: %v)",
						rel, path, allowedPrefixes)
					violations++
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk error: %v", err)
	}
	if violations > 0 {
		t.Fatalf("found %d HC-6 isolation violations; provider SDKs must be confined to their packages", violations)
	}
}

// findModuleRoot walks upward from the test's working directory looking
// for go.mod. Returns the directory containing go.mod, or an error.
func findModuleRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

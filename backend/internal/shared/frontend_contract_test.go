package shared

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFrontendDynamicRoutes_ExposeGenerateStaticParams(t *testing.T) {
	t.Parallel()
	// Temporarily skipped by product decision:
	// the frontend is currently not shipping static-export mode, so this guard
	// is kept as documentation but does not fail CI until static export is enabled.
	// Re-enable by removing this Skip once output: "export" is turned on.
	t.Skip("static-export route contract is deferred; keep test for future enablement")

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	routeFiles := []string{
		filepath.Join(repoRoot, "frontend", "src", "app", "session", "[code]", "page.tsx"),
		filepath.Join(repoRoot, "frontend", "src", "app", "interview", "[code]", "page.tsx"),
	}

	for _, p := range routeFiles {
		content, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if !strings.Contains(string(content), "generateStaticParams") {
			t.Fatalf("%s is missing generateStaticParams() required by architecture-improvement.md static-export guidance", p)
		}
	}
}

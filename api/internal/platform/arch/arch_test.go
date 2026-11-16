package arch

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const modulePrefix = "github.com/hectorruiz23/temper/api"

func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "api", "go.mod")); err != nil {
		t.Fatalf("repository root %s does not contain api/go.mod: %v", root, err)
	}

	return root
}

func walkDomainPackages(t *testing.T, apiRoot string, visit func(t *testing.T, directory string)) {
	t.Helper()

	err := filepath.WalkDir(filepath.Join(apiRoot, "internal"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if path != filepath.Join(apiRoot, "internal") && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "testdata") {
			return fs.SkipDir
		}
		if filepath.Base(path) == "domain" {
			visit(t, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal packages: %v", err)
	}
}

func packageImports(t *testing.T, directory string) []string {
	t.Helper()

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read %s: %v", directory, err)
	}

	var imports []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		file := filepath.Join(directory, entry.Name())
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, spec := range parsed.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", file, err)
			}
			imports = append(imports, importPath)
		}
	}

	return imports
}

func isStandardLibrary(importPath string) bool {
	first := strings.Split(importPath, "/")[0]
	return !strings.Contains(first, ".")
}

func TestDomainPackagesImportOnlyStandardLibraryAndKernel(t *testing.T) {
	apiRoot := filepath.Join(repoRoot(t), "api")

	walkDomainPackages(t, apiRoot, func(t *testing.T, directory string) {
		for _, importPath := range packageImports(t, directory) {
			if isStandardLibrary(importPath) {
				continue
			}
			if importPath == modulePrefix+"/internal/kernel" {
				continue
			}
			t.Errorf("%s imports %s, want only the standard library and %s/internal/kernel", directory, importPath, modulePrefix)
		}
	})
}

func TestDomainPackagesDoNotImportApp(t *testing.T) {
	apiRoot := filepath.Join(repoRoot(t), "api")

	walkDomainPackages(t, apiRoot, func(t *testing.T, directory string) {
		for _, importPath := range packageImports(t, directory) {
			if strings.Contains(importPath, "/app/") || strings.HasSuffix(importPath, "/app") {
				t.Errorf("%s imports %s, want no app package from a domain package", directory, importPath)
			}
		}
	})
}

func TestSlicesHaveDomainAppAndAdapters(t *testing.T) {
	internalRoot := filepath.Join(repoRoot(t), "api", "internal")

	entries, err := os.ReadDir(internalRoot)
	if err != nil {
		t.Fatalf("read %s: %v", internalRoot, err)
	}

	reserved := map[string]bool{
		"kernel":     true,
		"platform":   true,
		"mcp":        true,
		"acceptance": true,
	}

	for _, entry := range entries {
		if !entry.IsDir() || reserved[entry.Name()] || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		for _, required := range []string{"domain", "app", "adapters"} {
			directory := filepath.Join(internalRoot, entry.Name(), required)
			info, err := os.Stat(directory)
			if err != nil || !info.IsDir() {
				t.Errorf("slice %s has no %s directory", entry.Name(), required)
			}
		}
	}
}

func TestMigrationsContainNoTriggers(t *testing.T) {
	migrationsRoot := filepath.Join(repoRoot(t), "api", "migrations")
	trigger := regexp.MustCompile(`(?i)create\s+trigger`)

	entries, err := os.ReadDir(migrationsRoot)
	if err != nil {
		t.Fatalf("read %s: %v", migrationsRoot, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(migrationsRoot, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		if trigger.Match(contents) {
			t.Errorf("migration %s contains a trigger", entry.Name())
		}
	}
}

func TestRepositoryRootHasOnlyAllowedFiles(t *testing.T) {
	root := repoRoot(t)
	allowed := map[string]bool{
		"README.md":       true,
		"AGENTS.md":       true,
		"SELF-HOSTING.md": true,
		"LICENSE":         true,
		".gitignore":      true,
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read repository root: %v", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == ".git" || strings.HasSuffix(name, ".local.md") {
			continue
		}
		if !allowed[name] {
			t.Errorf("repository root holds unexpected file %s", name)
		}
	}
}

func resolveBase(t *testing.T, root string) (string, bool) {
	t.Helper()

	for _, ref := range []string{"origin/main", "main"} {
		command := exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
		if err := command.Run(); err == nil {
			return ref, true
		}
	}

	return "", false
}

func TestMigrationsAreFrozen(t *testing.T) {
	root := repoRoot(t)

	base, ok := resolveBase(t, root)
	if !ok {
		t.Log("skipped: no base ref")
		return
	}

	command := exec.Command("git", "-C", root, "diff", "--name-status", base+"...HEAD", "--", "api/migrations")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git diff against %s: %v: %s", base, err, output)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		status := strings.Fields(line)[0]
		if strings.HasPrefix(status, "M") || strings.HasPrefix(status, "D") {
			t.Errorf("migration changed against %s: %s", base, line)
		}
	}
}

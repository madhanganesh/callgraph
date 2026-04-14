package lang

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Python implements Language for Python source files.
type Python struct{}

// pyRootMarkers are files that indicate a Python project root, checked in order.
var pyRootMarkers = []string{
	"pyproject.toml",
	"setup.py",
	"setup.cfg",
	"requirements.txt",
}

func (Python) FindRoot(file string) string {
	dir := filepath.Dir(file)
	for {
		for _, marker := range pyRootMarkers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Dir(file)
		}
		dir = parent
	}
}

func (Python) RelPkg(filePath, root string) string {
	dir := filepath.Dir(filePath)
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." {
		// File is at project root — use the filename without extension.
		base := filepath.Base(filePath)
		return strings.TrimSuffix(base, ".py")
	}
	return strings.ReplaceAll(rel, string(filepath.Separator), ".")
}

// pyFuncRe matches Python function and method definitions.
//
//	def foo(...)       — plain function
//	async def bar(...) — async function
var pyFuncRe = regexp.MustCompile(`^[ \t]*(async\s+)?def\s+(\w+)\s*\(`)

func (Python) EnclosingFunc(src []byte, cursorLine int) (line, col int) {
	lines := strings.Split(string(src), "\n")
	// Walk backwards from the cursor to find the nearest def.
	for i := cursorLine - 1; i >= 0; i-- {
		m := pyFuncRe.FindStringSubmatchIndex(lines[i])
		if m == nil {
			continue
		}
		// m[4], m[5] is the submatch for the function name (\w+).
		nameStart := m[4]
		return i + 1, nameStart + 1 // 1-based
	}
	return 0, 0
}

func (Python) LSPCommand() []string {
	if _, err := exec.LookPath("pyright-langserver"); err == nil {
		return []string{"pyright-langserver", "--stdio"}
	}
	return []string{"pylsp"}
}

func (Python) LanguageID() string {
	return "python"
}

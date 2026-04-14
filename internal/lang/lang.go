package lang

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/madhanganesh/callgraph/internal/classify"
)

// Language abstracts the language-specific operations needed to build a call
// graph. Each supported language provides its own implementation.
type Language interface {
	// FindRoot walks up from the file's directory to find the project root
	// (e.g. go.mod, pyproject.toml, Cargo.toml).
	FindRoot(file string) string

	// RelPkg returns a display-friendly relative package/module path from the
	// project root to the file's directory.
	RelPkg(file, root string) string

	// EnclosingFunc finds the function that encloses the given 1-based line
	// in src and returns its name position (1-based line, col).
	// Returns (0, 0) if no enclosing function is found.
	EnclosingFunc(src []byte, line int) (funcLine, funcCol int)

	// LSPCommand returns the command name and arguments to start the language
	// server (e.g. ["gopls", "serve"] or ["pyright-langserver", "--stdio"]).
	LSPCommand() []string

	// LanguageID returns the LSP language identifier (e.g. "go", "python", "rust").
	LanguageID() string

	// ClassifyRules returns the default classification rules for this language.
	// Return nil if classification is not yet supported.
	ClassifyRules() []classify.Rule

	// ThreadSpawnPattern returns a regex that matches a call-site line when the
	// call is made in a new thread/goroutine/async task (e.g. "^\\s*go " for Go).
	// Return nil if thread detection is not supported.
	ThreadSpawnPattern() *regexp.Regexp
}

// Detect returns the Language implementation for the given file based on its
// extension.
func Detect(file string) (Language, error) {
	ext := strings.ToLower(filepath.Ext(file))
	switch ext {
	case ".go":
		return GoLang{}, nil
	case ".py":
		return Python{}, nil
	case ".rs":
		return Rust{}, nil
	default:
		return nil, fmt.Errorf("unsupported file type: %s", ext)
	}
}

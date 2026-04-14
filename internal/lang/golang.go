package lang

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// GoLang implements Language for Go source files.
type GoLang struct{}

func (GoLang) FindRoot(file string) string {
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Dir(file)
		}
		dir = parent
	}
}

func (GoLang) RelPkg(filePath, moduleRoot string) string {
	dir := filepath.Dir(filePath)
	rel, err := filepath.Rel(moduleRoot, dir)
	if err != nil || rel == "." {
		// File is in the module root — use the package declaration.
		return goPkgName(filePath)
	}
	return strings.ReplaceAll(rel, string(filepath.Separator), ".")
}

func (GoLang) EnclosingFunc(src []byte, cursorLine int) (line, col int) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		return 0, 0
	}

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		start := fset.Position(fn.Pos())
		end := fset.Position(fn.End())

		if cursorLine >= start.Line && cursorLine <= end.Line {
			namePos := fset.Position(fn.Name.Pos())
			return namePos.Line, namePos.Column
		}
	}
	return 0, 0
}

func (GoLang) LSPCommand() []string {
	return []string{"gopls", "serve"}
}

func (GoLang) LanguageID() string {
	return "go"
}

// goPkgName reads just the package declaration from a Go source file.
func goPkgName(filePath string) string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, parser.PackageClauseOnly)
	if err != nil {
		return ""
	}
	return f.Name.Name
}

package lang

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/madhanganesh/callgraph/internal/classify"
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

// goThreadSpawn matches a line whose first statement begins with `go ` — i.e.
// the call on that line starts a new goroutine. `go func() { ... }()` and
// `go foo()` both match.
var goThreadSpawn = regexp.MustCompile(`^\s*go\s+\S`)

func (GoLang) ThreadSpawnPattern() *regexp.Regexp {
	return goThreadSpawn
}

// Classification rules match the qualified target string "<pkg>.<Name>" that
// callgraph builds from gopls's CallHierarchyItem (e.g. "net/http.Get",
// "database/sql.Exec"). We don't see receiver types, so DB/API rules key on
// package path plus a common method-name allowlist.
func (GoLang) ClassifyRules() []classify.Rule {
	return []classify.Rule{
		// HTTP clients (stdlib and popular libraries).
		classify.MustRule(classify.KindAPI, `^net/http\.(Get|Post|Head|PostForm|Do|NewRequest|NewRequestWithContext)$`),
		classify.MustRule(classify.KindAPI, `^github\.com/go-resty/resty`),
		classify.MustRule(classify.KindAPI, `^github\.com/hashicorp/go-retryablehttp`),

		// Database drivers / ORMs — any call into these packages counts.
		classify.MustRule(classify.KindDB, `^database/sql\.(Query|QueryRow|Exec|Prepare|Ping|Begin|Commit|Rollback)(Context)?$`),
		classify.MustRule(classify.KindDB, `^gorm\.io/gorm`),
		classify.MustRule(classify.KindDB, `^github\.com/jackc/pgx`),
		classify.MustRule(classify.KindDB, `^github\.com/jmoiron/sqlx`),
		classify.MustRule(classify.KindDB, `^go\.mongodb\.org/mongo-driver`),
		classify.MustRule(classify.KindDB, `^github\.com/redis/go-redis`),
	}
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

package lang

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/madhanganesh/callgraph/internal/classify"
)

// Rust implements Language for Rust source files.
type Rust struct{}

func (Rust) FindRoot(file string) string {
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "Cargo.toml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Dir(file)
		}
		dir = parent
	}
}

func (Rust) RelPkg(filePath, root string) string {
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return filepath.Base(filePath)
	}
	// Strip "src/" prefix and ".rs" suffix → "handlers/order" → "handlers.order"
	rel = strings.TrimPrefix(rel, "src"+string(filepath.Separator))
	rel = strings.TrimSuffix(rel, ".rs")
	// "mod" and "lib" are conventional names, use parent directory instead.
	if rel == "mod" || rel == "lib" || rel == "main" {
		dir := filepath.Dir(filePath)
		dirRel, err := filepath.Rel(root, dir)
		if err != nil || dirRel == "." || dirRel == "src" {
			return crateName(root)
		}
		rel = strings.TrimPrefix(dirRel, "src"+string(filepath.Separator))
	}
	return strings.ReplaceAll(rel, string(filepath.Separator), "::")
}

// rustFuncRe matches Rust function definitions.
//
//	fn foo(...)
//	pub fn bar(...)
//	pub(crate) async fn baz(...)
var rustFuncRe = regexp.MustCompile(`^[ \t]*(pub(\([^)]*\))?\s+)?(async\s+)?fn\s+(\w+)`)

func (Rust) EnclosingFunc(src []byte, cursorLine int) (line, col int) {
	lines := strings.Split(string(src), "\n")
	braceDepth := 0

	// Walk backwards from cursor, tracking brace depth so we skip nested fns.
	for i := cursorLine - 1; i >= 0; i-- {
		l := lines[i]
		// Count braces on this line (simplified — doesn't handle strings/comments).
		for j := len(l) - 1; j >= 0; j-- {
			switch l[j] {
			case '}':
				braceDepth++
			case '{':
				braceDepth--
			}
		}

		if braceDepth > 0 {
			continue // inside a nested block above the cursor
		}

		m := rustFuncRe.FindStringSubmatchIndex(l)
		if m == nil {
			continue
		}
		// m[8], m[9] is the submatch for the function name (\w+).
		nameStart := m[8]
		return i + 1, nameStart + 1
	}
	return 0, 0
}

func (Rust) LSPCommand() []string {
	return []string{"rust-analyzer"}
}

func (Rust) LanguageID() string {
	return "rust"
}

// rustThreadSpawn matches call-site lines that spawn a thread or async task.
// Covers std::thread::spawn, thread::spawn, tokio::spawn, async_std::task::spawn,
// rayon::spawn, and tokio::task::spawn / spawn_blocking.
var rustThreadSpawn = regexp.MustCompile(`\b((std::)?thread::spawn|tokio::(task::)?spawn(_blocking)?|async_std::task::spawn|rayon::spawn)\s*\(`)

func (Rust) ThreadSpawnPattern() *regexp.Regexp { return rustThreadSpawn }

// Rules match the qualified target "<crate>::<name>" — rust-analyzer puts the
// containing module path in CallHierarchyItem.Detail.
func (Rust) ClassifyRules() []classify.Rule {
	return []classify.Rule{
		// HTTP clients.
		classify.MustRule(classify.KindAPI, `^reqwest::`),
		classify.MustRule(classify.KindAPI, `^hyper::`),
		classify.MustRule(classify.KindAPI, `^surf::`),
		classify.MustRule(classify.KindAPI, `^isahc::`),
		classify.MustRule(classify.KindAPI, `^ureq::`),

		// Databases.
		classify.MustRule(classify.KindDB, `^sqlx::`),
		classify.MustRule(classify.KindDB, `^diesel::`),
		classify.MustRule(classify.KindDB, `^tokio_postgres::`),
		classify.MustRule(classify.KindDB, `^postgres::`),
		classify.MustRule(classify.KindDB, `^rusqlite::`),
		classify.MustRule(classify.KindDB, `^mongodb::`),
		classify.MustRule(classify.KindDB, `^redis::`),
		classify.MustRule(classify.KindDB, `^sea_orm::`),
	}
}

// crateName extracts the crate name from Cargo.toml, falling back to the
// directory name.
func crateName(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "Cargo.toml"))
	if err != nil {
		return filepath.Base(root)
	}
	// Quick-and-dirty: look for name = "foo" under [package].
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name") && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			name := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			if name != "" {
				return name
			}
		}
	}
	return filepath.Base(root)
}

// Package summarize extracts the function under cursor and pipes it to an
// external LLM CLI for a short natural-language summary.
package summarize

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/madhanganesh/callgraph/internal/lang"
	"github.com/madhanganesh/callgraph/internal/lsp"
)

const promptHeader = "Summarize what this function does in 2-3 sentences. " +
	"Be concrete; mention side effects, IO, and error paths if any.\n\n"

// Result is what Run returns: either a Summary, or — when the cursor is on
// an interface method with multiple real implementations — a list of impls
// for the caller to pick from.
type Result struct {
	Summary         string `json:"summary,omitempty"`
	Implementations []Impl `json:"implementations,omitempty"`
}

// Impl is a concrete implementation candidate.
type Impl struct {
	Name string `json:"name"`
	Pkg  string `json:"pkg"`
	File string `json:"file"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
}

// Run resolves the symbol under cursor (file:line:col), follows it to a
// concrete definition (auto-picking through interfaces when only one real
// impl exists), extracts that function's body, and pipes it to llmCmd.
func Run(file string, line, col int, llmCmd []string) (*Result, error) {
	absFile, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	language, err := lang.Detect(absFile)
	if err != nil {
		return nil, err
	}

	// Position-level cache: skips LSP on a repeat keypress at the same
	// cursor in an unmodified file.
	srcBytes, err := os.ReadFile(absFile)
	if err != nil {
		return nil, err
	}
	posKey := positionCacheKey(absFile, line, col, srcBytes, llmCmd)
	if cached, ok := readResultCache(posKey); ok {
		return cached, nil
	}

	defFile, defLine, impls, err := resolve(absFile, language, line, col)
	if err != nil {
		return nil, err
	}
	if len(impls) > 1 {
		res := &Result{Implementations: impls}
		writeResultCache(posKey, res)
		return res, nil
	}

	src, err := os.ReadFile(defFile)
	if err != nil {
		return nil, err
	}
	endLine, err := functionEnd(defFile, src, defLine)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(src), "\n")
	if endLine > len(lines) {
		endLine = len(lines)
	}
	body := strings.Join(lines[defLine-1:endLine], "\n")

	if len(llmCmd) == 0 {
		llmCmd = []string{"claude", "-p"}
	}
	prompt := promptHeader + body
	key := cacheKey(llmCmd, prompt)
	if cached, ok := readCache(key); ok {
		return &Result{Summary: cached}, nil
	}

	cmd := exec.Command(llmCmd[0], llmCmd[1:]...)
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %v\n%s", llmCmd[0], err, stderr.String())
	}
	out := strings.TrimRight(stdout.String(), "\n") + "\n"
	writeCache(key, out)
	res := &Result{Summary: out}
	writeResultCache(posKey, res)
	return res, nil
}

// positionCacheKey hashes (file, cursor, file-content, llmCmd). A file edit
// or model change busts the entry; same keypress on an unmodified file hits
// and skips LSP entirely.
func positionCacheKey(file string, line, col int, src []byte, llmCmd []string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%d\x00%d\x00", file, line, col)
	h.Write(src)
	h.Write([]byte("\x00"))
	h.Write([]byte(strings.Join(llmCmd, "\x00")))
	return "pos-" + hex.EncodeToString(h.Sum(nil))
}

func readResultCache(key string) (*Result, bool) {
	data, ok := readCache(key)
	if !ok {
		return nil, false
	}
	var r Result
	if err := json.Unmarshal([]byte(data), &r); err != nil {
		return nil, false
	}
	return &r, true
}

func writeResultCache(key string, r *Result) {
	b, err := json.Marshal(r)
	if err != nil {
		return
	}
	writeCache(key, string(b))
}

// resolve picks the definition to summarize. It returns either:
//   - (defFile, defLine, nil, nil) when there's a single target (no impls,
//     or exactly one real impl after filtering mocks/tests), or
//   - ("", 0, impls, nil) when there are multiple real impls and the user
//     must pick.
func resolve(file string, language lang.Language, line, col int) (string, int, []Impl, error) {
	root := language.FindRoot(file)
	client, err := lsp.NewClient(root, language.LSPCommand())
	if err != nil {
		return "", 0, nil, err
	}
	defer client.Close()

	src, err := os.ReadFile(file)
	if err != nil {
		return "", 0, nil, err
	}
	uri := lsp.FileURI(file)
	if err := client.DidOpen(uri, language.LanguageID(), string(src)); err != nil {
		return "", 0, nil, err
	}

	items, err := client.PrepareCallHierarchy(uri, line-1, col-1)
	if err != nil {
		return "", 0, nil, err
	}
	if len(items) == 0 {
		return "", 0, nil, fmt.Errorf("no symbol resolved at %s:%d:%d", file, line, col)
	}
	item := items[0]
	itemFile := lsp.URIToFile(item.URI)
	itemLine := item.SelectionRange.Start.Line + 1

	// If the cursor's own item already points at a concrete function body
	// we can extract, use it. Implementation lookup is only needed when the
	// item is an interface method (extraction will fail).
	if itemSrc, err := os.ReadFile(itemFile); err == nil {
		if _, err := functionEnd(itemFile, itemSrc, itemLine); err == nil {
			return itemFile, itemLine, nil, nil
		}
	}

	locs, _ := client.Implementation(item.URI, item.SelectionRange.Start.Line, item.SelectionRange.Start.Character)
	var impls []Impl
	for _, loc := range locs {
		f := lsp.URIToFile(loc.URI)
		// Drop the item itself when the LSP echoes it back.
		if f == itemFile && loc.Range.Start.Line+1 == itemLine {
			continue
		}
		pkg := language.RelPkg(f, root)
		if isMockOrTest(f, pkg) {
			continue
		}
		impls = append(impls, Impl{
			Name: item.Name,
			Pkg:  pkg,
			File: f,
			Line: loc.Range.Start.Line + 1,
			Col:  loc.Range.Start.Character + 1,
		})
	}

	switch len(impls) {
	case 0:
		return itemFile, itemLine, nil, nil
	case 1:
		return impls[0].File, impls[0].Line, nil, nil
	default:
		return "", 0, impls, nil
	}
}

func isMockOrTest(file, pkg string) bool {
	lf := strings.ToLower(file)
	lp := strings.ToLower(pkg)
	return strings.HasSuffix(lf, "_test.go") ||
		strings.Contains(lf, "/mock") ||
		strings.Contains(lf, "/fake") ||
		strings.Contains(lp, "mock") ||
		strings.Contains(lp, "fake")
}

// cacheKey hashes the LLM command + prompt so a different model or modified
// function body produces a fresh entry.
func cacheKey(llmCmd []string, prompt string) string {
	h := sha256.New()
	h.Write([]byte(strings.Join(llmCmd, "\x00")))
	h.Write([]byte("\x00"))
	h.Write([]byte(prompt))
	return hex.EncodeToString(h.Sum(nil))
}

func cacheDir() string {
	if d := os.Getenv("CALLGRAPH_CACHE_DIR"); d != "" {
		return d
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "callgraph", "summaries")
}

func readCache(key string) (string, bool) {
	dir := cacheDir()
	if dir == "" {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(dir, key))
	if err != nil {
		return "", false
	}
	return string(data), true
}

func writeCache(key, content string) {
	dir := cacheDir()
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, key), []byte(content), 0o644)
}

// functionEnd returns the 1-based line number where the function starting at
// startLine ends.
func functionEnd(file string, src []byte, startLine int) (int, error) {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".go":
		return goFuncEnd(file, src, startLine)
	case ".py":
		return pyFuncEnd(src, startLine), nil
	case ".rs":
		return braceFuncEnd(src, startLine), nil
	}
	return startLine, fmt.Errorf("unsupported file type for summarize: %s", file)
}

func goFuncEnd(file string, src []byte, startLine int) (int, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, src, parser.SkipObjectResolution)
	if err != nil {
		return 0, err
	}
	for _, decl := range f.Decls {
		pos := fset.Position(decl.Pos())
		if pos.Line == startLine {
			return fset.Position(decl.End()).Line, nil
		}
	}
	return startLine, fmt.Errorf("no top-level decl at line %d", startLine)
}

// pyFuncEnd walks forward from the def line, returning the last line at an
// indentation strictly greater than the def's own indent (or the def line if
// the body is empty).
func pyFuncEnd(src []byte, startLine int) int {
	lines := strings.Split(string(src), "\n")
	if startLine > len(lines) {
		return startLine
	}
	defIndent := leadingIndent(lines[startLine-1])
	end := startLine
	for i := startLine; i < len(lines); i++ {
		l := lines[i]
		if strings.TrimSpace(l) == "" {
			continue
		}
		if leadingIndent(l) <= defIndent {
			break
		}
		end = i + 1
	}
	return end
}

func leadingIndent(l string) int {
	n := 0
	for _, r := range l {
		if r == ' ' || r == '\t' {
			n++
		} else {
			break
		}
	}
	return n
}

// braceFuncEnd walks forward from the fn line counting braces, returning the
// line containing the matching closing brace.
func braceFuncEnd(src []byte, startLine int) int {
	lines := strings.Split(string(src), "\n")
	depth := 0
	seen := false
	for i := startLine - 1; i < len(lines); i++ {
		for _, c := range lines[i] {
			switch c {
			case '{':
				depth++
				seen = true
			case '}':
				depth--
				if seen && depth == 0 {
					return i + 1
				}
			}
		}
	}
	return len(lines)
}

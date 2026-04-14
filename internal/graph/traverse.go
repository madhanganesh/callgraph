package graph

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/madhanganesh/callgraph/internal/classify"
	"github.com/madhanganesh/callgraph/internal/lang"
	"github.com/madhanganesh/callgraph/internal/lsp"
)

// Node represents a function in a call tree. Either Callers (incoming) or
// Callees (outgoing) will be populated depending on traversal direction.
type Node struct {
	Name    string        `json:"name"`
	Pkg     string        `json:"pkg"`
	File    string        `json:"file"`
	Line    int           `json:"line"`
	Col     int           `json:"col,omitempty"`
	Kind    classify.Kind `json:"kind,omitempty"`
	Detail  string        `json:"detail,omitempty"`
	Callers         []*Node `json:"callers,omitempty"`
	Callees         []*Node `json:"callees,omitempty"`
	Implementations []*Node `json:"implementations,omitempty"`
}

// BuildCallerTree builds the upward call graph starting from the function at
// the given file position. Line and col are 1-based (user convention); they
// are converted to 0-based internally for LSP.
func BuildCallerTree(client *lsp.Client, language lang.Language, file, root string, line, col, maxDepth int) (*Node, error) {
	uri := lsp.FileURI(file)

	content, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}

	// Find the enclosing function for the given line. This lets the user invoke
	// the tool from anywhere inside a function body, not just the declaration.
	funcLine, funcCol := language.EnclosingFunc(content, line)
	if funcLine > 0 {
		line = funcLine
		col = funcCol
	}

	if err := client.DidOpen(uri, language.LanguageID(), string(content)); err != nil {
		return nil, fmt.Errorf("didOpen: %w", err)
	}

	// gopls may still be indexing after didOpen. Retry a few times.
	var items []lsp.CallHierarchyItem
	for attempt := 0; attempt < 10; attempt++ {
		items, err = client.PrepareCallHierarchy(uri, line-1, col-1)
		if err == nil && len(items) > 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		return nil, fmt.Errorf("prepareCallHierarchy: %w", err)
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no function found at %s:%d:%d", file, line, col)
	}

	item := items[0]
	itemFile := lsp.URIToFile(item.URI)
	rootNode := &Node{
		Name: item.Name,
		Pkg:  language.RelPkg(itemFile, root),
		File: itemFile,
		Line: item.SelectionRange.Start.Line + 1,
	}

	visited := make(map[string]bool)
	if err := walkCallers(client, language, item, rootNode, maxDepth, root, visited); err != nil {
		return nil, err
	}

	return rootNode, nil
}

// BuildCalleeTree builds the outgoing call graph (what this function calls)
// starting from the function at the given file position. Line and col are
// 1-based; converted to 0-based internally for LSP.
func BuildCalleeTree(client *lsp.Client, language lang.Language, file, root string, line, col, maxDepth int) (*Node, error) {
	uri := lsp.FileURI(file)

	content, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}

	// Callee mode does NOT snap to enclosing function — when reading code,
	// the user points at a call-site token and wants the callees of THAT
	// symbol, not of the function they happen to be inside.

	if err := client.DidOpen(uri, language.LanguageID(), string(content)); err != nil {
		return nil, fmt.Errorf("didOpen: %w", err)
	}

	var items []lsp.CallHierarchyItem
	for attempt := 0; attempt < 10; attempt++ {
		items, err = client.PrepareCallHierarchy(uri, line-1, col-1)
		if err == nil && len(items) > 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		return nil, fmt.Errorf("prepareCallHierarchy: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no function found at %s:%d:%d", file, line, col)
	}

	item := items[0]
	itemFile := lsp.URIToFile(item.URI)
	rootNode := &Node{
		Name: item.Name,
		Pkg:  language.RelPkg(itemFile, root),
		File: itemFile,
		Line: item.SelectionRange.Start.Line + 1,
	}

	rules := language.ClassifyRules()
	threadPat := language.ThreadSpawnPattern()

	// Interface-method handling: filter out test/mock impls, then either
	// auto-descend into a single impl or hand back a picker list.
	if impls, err := resolveImplementations(client, language, item, root); err == nil && len(impls) > 0 {
		impls = filterRealImpls(impls)
		switch {
		case len(impls) > 1:
			rootNode.Implementations = impls
			return rootNode, nil
		case len(impls) == 1:
			// Auto-select: re-prepare hierarchy at the impl's location and
			// walk its callees as if the user had picked it.
			pick := impls[0]
			implItem, err := prepareItemAt(client, language, pick.File, pick.Line, pick.Col)
			if err == nil {
				rootNode.File = pick.File
				rootNode.Line = pick.Line
				rootNode.Pkg = pick.Pkg
				visited := make(map[string]bool)
				srcCache := make(map[string][]string)
				if err := walkCallees(client, language, implItem, rootNode, maxDepth, root, rules, threadPat, visited, srcCache); err != nil {
					return nil, err
				}
				return rootNode, nil
			}
			// Fall through on error — at worst we render the interface itself.
		}
	}

	visited := make(map[string]bool)
	srcCache := make(map[string][]string)
	if err := walkCallees(client, language, item, rootNode, maxDepth, root, rules, threadPat, visited, srcCache); err != nil {
		return nil, err
	}
	return rootNode, nil
}

// prepareItemAt re-runs prepareCallHierarchy at a known location. Used to
// auto-resolve a single picked implementation.
func prepareItemAt(client *lsp.Client, language lang.Language, file string, line, col int) (lsp.CallHierarchyItem, error) {
	uri := lsp.FileURI(file)
	if content, err := os.ReadFile(file); err == nil {
		_ = client.DidOpen(uri, language.LanguageID(), string(content))
	}
	var items []lsp.CallHierarchyItem
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		items, err = client.PrepareCallHierarchy(uri, line-1, col-1)
		if err == nil && len(items) > 0 {
			return items[0], nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		return lsp.CallHierarchyItem{}, err
	}
	return lsp.CallHierarchyItem{}, fmt.Errorf("no item at %s:%d:%d", file, line, col)
}

// filterRealImpls drops test fixtures and mock implementations — they
// dominate the picker but are rarely what the reader wants to inspect.
func filterRealImpls(impls []*Node) []*Node {
	out := impls[:0]
	for _, n := range impls {
		lower := strings.ToLower(n.File)
		if strings.HasSuffix(lower, "_test.go") ||
			strings.Contains(lower, "/mock") ||
			strings.Contains(lower, "/fake") ||
			strings.Contains(strings.ToLower(n.Pkg), "mock") ||
			strings.Contains(strings.ToLower(n.Pkg), "fake") {
			continue
		}
		out = append(out, n)
	}
	return out
}

// resolveImplementations returns implementation nodes if `item` represents
// an interface method with concrete implementations distinct from itself.
func resolveImplementations(client *lsp.Client, language lang.Language, item lsp.CallHierarchyItem, root string) ([]*Node, error) {
	locs, err := client.Implementation(item.URI, item.SelectionRange.Start.Line, item.SelectionRange.Start.Character)
	if err != nil {
		return nil, err
	}
	var out []*Node
	for _, loc := range locs {
		// Filter out the item itself — gopls sometimes echoes the interface
		// method as one of the "implementations".
		if loc.URI == item.URI && loc.Range.Start.Line == item.SelectionRange.Start.Line {
			continue
		}
		f := lsp.URIToFile(loc.URI)
		out = append(out, &Node{
			Name: item.Name,
			Pkg:  language.RelPkg(f, root),
			File: f,
			Line: loc.Range.Start.Line + 1,
			Col:  loc.Range.Start.Character + 1,
		})
	}
	return out, nil
}

// walkCallees recursively fetches outgoing calls and builds the callee tree.
// It also classifies each callee (API / DB / thread) based on the language's
// rules and the call-site source.
func walkCallees(client *lsp.Client, language lang.Language, item lsp.CallHierarchyItem, node *Node, depth int, root string, rules []classify.Rule, threadPat *regexp.Regexp, visited map[string]bool, srcCache map[string][]string) error {
	if depth <= 0 {
		return nil
	}

	key := fmt.Sprintf("%s:%d", item.URI, item.SelectionRange.Start.Line)
	if visited[key] {
		return nil
	}
	visited[key] = true

	// Empty results for outgoing calls mean "no more calls" — unlike incoming
	// calls, no need to retry on empty. Only retry on transient LSP errors.
	var calls []lsp.CallHierarchyOutgoingCall
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		calls, err = client.OutgoingCalls(item)
		if err != nil && isTransientLSPError(err) {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		break
	}
	if err != nil {
		return fmt.Errorf("outgoingCalls(%s): %w", item.Name, err)
	}

	// Cache source lines from the caller's file so we can check call-site text
	// for thread-spawn markers ("go f()" in Go, etc.).
	callerFile := lsp.URIToFile(item.URI)
	callerSrc, ok := srcCache[callerFile]
	if !ok {
		callerSrc = readLines(callerFile)
		srcCache[callerFile] = callerSrc
	}

	// Render in source order. gopls groups calls by target, not by call-site
	// position, so we sort by the earliest FromRange of each entry.
	sort.SliceStable(calls, func(i, j int) bool {
		a, b := earliestRange(calls[i].FromRanges), earliestRange(calls[j].FromRanges)
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Character < b.Character
	})

	for _, call := range calls {
		targetFile := lsp.URIToFile(call.To.URI)
		external := !isUnder(targetFile, root) || strings.Contains(targetFile, "/vendor/")

		qualified := qualifiedName(call.To)
		kind := classify.Classify(qualified, rules)

		// Thread detection inspects the call-site line, not the target name.
		if threadPat != nil && kind == classify.KindPlain {
			for _, r := range call.FromRanges {
				if r.Start.Line < len(callerSrc) && threadPat.MatchString(callerSrc[r.Start.Line]) {
					kind = classify.KindThread
					break
				}
			}
		}

		// Noise reduction: drop plain external calls (stdlib / vendor / deps).
		// They dilute the "what does this function do" signal. Classified
		// external calls (API/DB/thread) are kept as meaningful boundaries.
		if kind == classify.KindPlain && external {
			continue
		}

		// For files outside the module root (stdlib / deps), prefer the LSP
		// Detail string as the package label — it's authoritative.
		var pkg string
		if external && call.To.Detail != "" {
			pkg = detailPackage(call.To.Detail)
		} else {
			pkg = language.RelPkg(targetFile, root)
		}

		var detail string
		if kind == classify.KindAPI {
			for _, r := range call.FromRanges {
				if r.Start.Line < len(callerSrc) {
					if ep := extractEndpoint(callerSrc[r.Start.Line]); ep != "" {
						detail = ep
						break
					}
				}
			}
		} else if kind == classify.KindDB {
			for _, r := range call.FromRanges {
				if r.Start.Line < len(callerSrc) {
					// Glue up to 3 lines — SQL args often sit on the line
					// after `.Exec(`.
					end := r.Start.Line + 3
					if end > len(callerSrc) {
						end = len(callerSrc)
					}
					snippet := strings.Join(callerSrc[r.Start.Line:end], " ")
					if t := extractTable(snippet); t != "" {
						detail = t
						break
					}
				}
			}
		}

		calleeNode := &Node{
			Name:   call.To.Name,
			Pkg:    pkg,
			File:   targetFile,
			Line:   call.To.SelectionRange.Start.Line + 1,
			Col:    call.To.SelectionRange.Start.Character + 1,
			Kind:   kind,
			Detail: detail,
		}
		node.Callees = append(node.Callees, calleeNode)

		// Stop descent at API/DB/thread leaves and at external (stdlib/dep)
		// boundaries — the user cares about their own code's flow.
		if kind != classify.KindPlain || external {
			continue
		}
		if err := walkCallees(client, language, call.To, calleeNode, depth-1, root, rules, threadPat, visited, srcCache); err != nil {
			return err
		}
	}
	return nil
}

// qualifiedName builds the fully qualified name used for classification lookup.
// gopls puts the package path in Detail; it may be suffixed with " • file.go"
// which we strip.
func qualifiedName(item lsp.CallHierarchyItem) string {
	pkg := detailPackage(item.Detail)
	if pkg == "" {
		return item.Name
	}
	return pkg + "." + item.Name
}

// detailPackage returns the package portion of gopls's CallHierarchyItem.Detail
// field, which has the form "pkg/path • filename.go".
func detailPackage(detail string) string {
	if detail == "" {
		return ""
	}
	if i := strings.Index(detail, " • "); i >= 0 {
		return detail[:i]
	}
	return detail
}

// urlLiteralRe captures the first http/https URL literal on a line. Matches
// up to the first quote, ?, space, or + (concatenation marker), so
// `"https://api.example.com/price?item=" + item` yields `api.example.com/price`.
var urlLiteralRe = regexp.MustCompile(`["` + "`" + `]https?://([^"` + "`" + `?+\s]+)`)

// extractEndpoint returns host+path from the first URL string literal on the
// line, stripping scheme and any query part. Returns "" if no URL is found.
func extractEndpoint(line string) string {
	m := urlLiteralRe.FindStringSubmatch(line)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimRight(m[1], "/")
}

// earliestRange returns the lowest (line, character) Start position across
// the given ranges, or a zero Position if empty.
func earliestRange(ranges []lsp.Range) lsp.Position {
	var best lsp.Position
	first := true
	for _, r := range ranges {
		if first || r.Start.Line < best.Line ||
			(r.Start.Line == best.Line && r.Start.Character < best.Character) {
			best = r.Start
			first = false
		}
	}
	return best
}

// sqlTableRe captures the SQL verb and first table name from a string
// literal. Matches "INSERT INTO orders ...", "SELECT ... FROM users",
// "UPDATE t SET ...", "DELETE FROM orders WHERE ...".
var sqlTableRe = regexp.MustCompile(`(?i)\b(INSERT|SELECT|UPDATE|DELETE)\b[^"` + "`" + `]*?\b(?:FROM|INTO|UPDATE)\s+([a-zA-Z_][\w.]*)`)

// updateSimpleRe handles the common `UPDATE <table> SET ...` form that the
// main regex misses (UPDATE is both the verb and the keyword before the
// table, no FROM/INTO between them).
var updateSimpleRe = regexp.MustCompile(`(?i)\bUPDATE\s+([a-zA-Z_][\w.]*)\s+SET\b`)

// extractTable returns "VERB table" from the first SQL literal in line, or
// "" if no recognizable SQL is found.
func extractTable(line string) string {
	if m := sqlTableRe.FindStringSubmatch(line); len(m) == 3 {
		return strings.ToUpper(m[1]) + " " + m[2]
	}
	if m := updateSimpleRe.FindStringSubmatch(line); len(m) == 2 {
		return "UPDATE " + m[1]
	}
	return ""
}

// isUnder reports whether path lies within root (or equals it).
func isUnder(path, root string) bool {
	if path == root {
		return true
	}
	sep := string(os.PathSeparator)
	return strings.HasPrefix(path, strings.TrimSuffix(root, sep)+sep)
}

// readLines returns file contents split by newline, or nil on error. Callers
// must bounds-check against len(result).
func readLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(string(data), "\n")
}

// isTransientLSPError returns true for errors worth retrying: the server was
// still indexing, or cancelled our request mid-flight.
func isTransientLSPError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "-32801") || strings.Contains(msg, "-32802") ||
		strings.Contains(msg, "content modified") || strings.Contains(msg, "cancelled")
}

// walkCallers recursively fetches incoming calls and builds the caller tree.
func walkCallers(client *lsp.Client, language lang.Language, item lsp.CallHierarchyItem, node *Node, depth int, root string, visited map[string]bool) error {
	if depth <= 0 {
		return nil
	}

	// Deduplicate by URI + line to avoid infinite loops on recursive calls.
	key := fmt.Sprintf("%s:%d", item.URI, item.SelectionRange.Start.Line)
	if visited[key] {
		return nil
	}
	visited[key] = true

	// The server may still be indexing. Retry on transient errors, and a few
	// times on empty results — rust-analyzer sometimes returns success with
	// no callers while its index is still warming up.
	var calls []lsp.CallHierarchyIncomingCall
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		calls, err = client.IncomingCalls(item)
		if err != nil && isTransientLSPError(err) {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if err == nil && len(calls) == 0 && attempt < 3 {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		break
	}
	if err != nil {
		return fmt.Errorf("incomingCalls(%s): %w", item.Name, err)
	}

	for _, call := range calls {
		callerFile := lsp.URIToFile(call.From.URI)
		callerNode := &Node{
			Name: call.From.Name,
			Pkg:  language.RelPkg(callerFile, root),
			File: callerFile,
			Line: call.From.SelectionRange.Start.Line + 1,
		}
		node.Callers = append(node.Callers, callerNode)

		if err := walkCallers(client, language, call.From, callerNode, depth-1, root, visited); err != nil {
			return err
		}
	}

	return nil
}


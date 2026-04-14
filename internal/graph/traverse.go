package graph

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/madhanganesh/callgraph/internal/lang"
	"github.com/madhanganesh/callgraph/internal/lsp"
)

// Node represents a function in the caller tree.
type Node struct {
	Name    string  `json:"name"`
	Pkg     string  `json:"pkg"`
	File    string  `json:"file"`
	Line    int     `json:"line"`
	Callers []*Node `json:"callers,omitempty"`
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

	// The server may still be indexing and return ContentModified (-32801) or
	// RequestCancelled (-32802). Retry with backoff.
	var calls []lsp.CallHierarchyIncomingCall
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		calls, err = client.IncomingCalls(item)
		if err == nil || !isTransientLSPError(err) {
			break
		}
		time.Sleep(500 * time.Millisecond)
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


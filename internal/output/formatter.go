package output

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/madhanganesh/callgraph/internal/graph"
)

// maxLineWidth is the width at which package names get trimmed.
const maxLineWidth = 80

// FormatJSON returns the caller tree as indented JSON.
func FormatJSON(node *graph.Node) string {
	data, err := json.MarshalIndent(node, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(data)
}

// FormatTree returns a top-down tree showing every call path from root callers
// down to the target function. Each node shows funcName (package).
//
// Example output for CreateOrder called from two paths:
//
//	main (main)
//	  |__ startRouter (main)
//	        |__ GetRouter (router)
//	              |__ addOrderRoutes (routes)
//	                    |__ PlaceOrder (handlers)
//	                          |__ CreateOrder (orders)
//	TestPlaceOrder (orders_test)
//	  |__ CreateOrder (orders)
func FormatTree(node *graph.Node) string {
	// Collect all root-to-target paths by reversing the caller tree.
	var paths [][]pathEntry
	collectPaths(node, nil, &paths)

	// Merge paths into a tree printed top-down.
	var sb strings.Builder
	root := buildPrintTree(paths)
	for i, child := range root.children {
		writePrintNode(&sb, child, "", i == len(root.children)-1)
	}
	return sb.String()
}

type pathEntry struct {
	name string
	pkg  string
	file string
	line int
}

// collectPaths walks the caller tree (target -> callers) and collects every
// complete path reversed to caller -> ... -> target order.
func collectPaths(node *graph.Node, suffix []pathEntry, out *[][]pathEntry) {
	entry := pathEntry{name: node.Name, pkg: node.Pkg, file: node.File, line: node.Line}
	current := append([]pathEntry{entry}, suffix...)

	if len(node.Callers) == 0 {
		path := make([]pathEntry, len(current))
		copy(path, current)
		*out = append(*out, path)
		return
	}

	for _, caller := range node.Callers {
		collectPaths(caller, current, out)
	}
}

// printNode is an intermediate tree used for merging shared prefixes before
// rendering.
type printNode struct {
	entry    pathEntry
	children []*printNode
}

func buildPrintTree(paths [][]pathEntry) *printNode {
	root := &printNode{}
	for _, path := range paths {
		insertPath(root, path)
	}
	return root
}

func insertPath(node *printNode, path []pathEntry) {
	if len(path) == 0 {
		return
	}

	head := path[0]
	rest := path[1:]

	// Look for an existing child with the same function.
	for _, child := range node.children {
		if child.entry.name == head.name &&
			child.entry.file == head.file &&
			child.entry.line == head.line {
			insertPath(child, rest)
			return
		}
	}

	// New branch.
	child := &printNode{entry: head}
	node.children = append(node.children, child)
	insertPath(child, rest)
}

func writePrintNode(sb *strings.Builder, node *printNode, prefix string, isLast bool) {
	pkg := node.entry.pkg
	label := fmt.Sprintf("%s (%s)", node.entry.name, pkg)

	var line string
	if prefix == "" {
		line = label
	} else {
		line = prefix + "|__ " + label
	}

	// Trim package name if the line exceeds maxLineWidth.
	if len(line) > maxLineWidth && len(pkg) > 4 {
		overflow := len(line) - maxLineWidth
		trimTo := len(pkg) - overflow - 3 // 3 for "..."
		if trimTo < 3 {
			trimTo = 3
		}
		pkg = pkg[:trimTo] + "..."
		label = fmt.Sprintf("%s (%s)", node.entry.name, pkg)
		if prefix == "" {
			line = label
		} else {
			line = prefix + "|__ " + label
		}
	}

	fmt.Fprintf(sb, "%s\n", line)

	// Build the prefix for children.
	var childPrefix string
	if prefix == "" {
		childPrefix = "  "
	} else if isLast {
		childPrefix = prefix + "    "
	} else {
		childPrefix = prefix + "|   "
	}

	for i, child := range node.children {
		writePrintNode(sb, child, childPrefix, i == len(node.children)-1)
	}
}

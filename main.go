package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/madhanganesh/callgraph/internal/graph"
	"github.com/madhanganesh/callgraph/internal/lang"
	"github.com/madhanganesh/callgraph/internal/lsp"
	"github.com/madhanganesh/callgraph/internal/output"
)

func main() {
	file := flag.String("file", "", "Absolute path to the source file")
	line := flag.Int("line", 0, "Line number (1-based)")
	col := flag.Int("col", 1, "Column number (1-based)")
	depth := flag.Int("depth", 5, "Max caller traversal depth")
	format := flag.String("format", "tree", "Output format: json or tree")
	flag.Parse()

	if *file == "" || *line == 0 {
		fmt.Fprintf(os.Stderr, "Usage: callgraph --file=<path> --line=<n> [--col=<n>] [--depth=<n>] [--format=json|tree]\n")
		os.Exit(1)
	}

	absFile, err := filepath.Abs(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	language, err := lang.Detect(absFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	rootDir := language.FindRoot(absFile)

	client, err := lsp.NewClient(rootDir, language.LSPCommand())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	tree, err := graph.BuildCallerTree(client, language, absFile, rootDir, *line, *col, *depth)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	switch *format {
	case "json":
		fmt.Println(output.FormatJSON(tree))
	case "tree":
		fmt.Print(output.FormatTree(tree))
	default:
		fmt.Fprintf(os.Stderr, "unknown format: %s (use json or tree)\n", *format)
		os.Exit(1)
	}
}

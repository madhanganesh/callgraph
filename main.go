package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/madhanganesh/callgraph/internal/graph"
	"github.com/madhanganesh/callgraph/internal/lang"
	"github.com/madhanganesh/callgraph/internal/lsp"
	"github.com/madhanganesh/callgraph/internal/output"
	"github.com/madhanganesh/callgraph/internal/summarize"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "summarize" {
		runSummarize(os.Args[2:])
		return
	}

	file := flag.String("file", "", "Absolute path to the source file")
	line := flag.Int("line", 0, "Line number (1-based)")
	col := flag.Int("col", 1, "Column number (1-based)")
	depth := flag.Int("depth", 5, "Max traversal depth")
	format := flag.String("format", "tree", "Output format: json or tree")
	direction := flag.String("direction", "callers", "Traversal direction: callers (incoming) or callees (outgoing)")
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

	var tree *graph.Node
	switch *direction {
	case "callers":
		tree, err = graph.BuildCallerTree(client, language, absFile, rootDir, *line, *col, *depth)
	case "callees":
		tree, err = graph.BuildCalleeTree(client, language, absFile, rootDir, *line, *col, *depth)
	default:
		fmt.Fprintf(os.Stderr, "unknown direction: %s (use callers or callees)\n", *direction)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	switch *format {
	case "json":
		fmt.Println(output.FormatJSON(tree))
	case "tree":
		if *direction == "callees" {
			fmt.Print(output.FormatCalleeTree(tree))
		} else {
			fmt.Print(output.FormatTree(tree))
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown format: %s (use json or tree)\n", *format)
		os.Exit(1)
	}
}

func runSummarize(args []string) {
	fs := flag.NewFlagSet("summarize", flag.ExitOnError)
	file := fs.String("file", "", "Absolute path to the source file")
	line := fs.Int("line", 0, "Line number (1-based)")
	col := fs.Int("col", 1, "Column number (1-based)")
	llmCmd := fs.String("llm-cmd", "", "LLM command (default: 'claude -p'). Space-separated.")
	_ = fs.Parse(args)

	if *file == "" || *line == 0 {
		fmt.Fprintf(os.Stderr, "Usage: callgraph summarize --file=<path> --line=<n> [--col=<n>] [--llm-cmd='claude -p']\n")
		os.Exit(1)
	}
	var cmd []string
	if *llmCmd != "" {
		cmd = strings.Fields(*llmCmd)
	}
	res, err := summarize.Run(*file, *line, *col, cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	b, _ := json.Marshal(res)
	fmt.Println(string(b))
}

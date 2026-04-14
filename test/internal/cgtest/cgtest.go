// Package cgtest provides shared helpers for callgraph integration tests.
package cgtest

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

// Node mirrors graph.Node for decoding JSON output.
type Node struct {
	Name    string  `json:"name"`
	Pkg     string  `json:"pkg"`
	File    string  `json:"file"`
	Line    int     `json:"line"`
	Callers []*Node `json:"callers,omitempty"`
}

// BuildBinary compiles the callgraph CLI into dir and returns the binary path.
// Suitable for calling from TestMain where no *testing.T is available.
func BuildBinary(dir string) (string, error) {
	out := filepath.Join(dir, "callgraph")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = RepoRoot()
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build callgraph: %w\n%s", err, b)
	}
	return out, nil
}

// RepoRoot returns the absolute path to the repository root.
func RepoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

// Run invokes the binary with --format=json and decodes the tree.
func Run(t *testing.T, bin, file string, line, col int) *Node {
	t.Helper()
	args := []string{
		"--file=" + file,
		"--line=" + strconv.Itoa(line),
		"--col=" + strconv.Itoa(col),
		"--format=json",
	}
	out, err := exec.Command(bin, args...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("callgraph failed: %v\nstderr: %s", err, ee.Stderr)
		}
		t.Fatalf("callgraph failed: %v", err)
	}
	var n Node
	if err := json.Unmarshal(out, &n); err != nil {
		t.Fatalf("decode json: %v\n%s", err, out)
	}
	return &n
}

// CollectCallerNames returns a set of every caller name transitively reachable
// from the given root (excluding the root itself).
func CollectCallerNames(root *Node) map[string]bool {
	seen := map[string]bool{}
	var walk func(*Node)
	walk = func(n *Node) {
		for _, c := range n.Callers {
			seen[c.Name] = true
			walk(c)
		}
	}
	walk(root)
	return seen
}

// SkipIfMissing skips the test if `name` is not on $PATH.
func SkipIfMissing(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not found in PATH; skipping", name)
	}
}


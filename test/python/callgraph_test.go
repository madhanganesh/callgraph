package pythontests

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/madhanganesh/callgraph/test/internal/cgtest"
)

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "callgraph-py-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	bin, err := cgtest.BuildBinary(dir)
	if err != nil {
		panic(err)
	}
	binary = bin
	os.Exit(m.Run())
}

func build(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("pyright-langserver"); err != nil {
		if _, err := exec.LookPath("pylsp"); err != nil {
			t.Skip("no Python LSP (pyright-langserver or pylsp) in PATH")
		}
	}
	return binary
}

func TestSimple(t *testing.T) {
	bin := build(t)
	file, _ := filepath.Abs("testdata/simple/main.py")
	// Line 5: `def compute(n):`
	root := cgtest.Run(t, bin, file, 5, 5)
	if root.Name != "compute" {
		t.Fatalf("root name: got %q want %q", root.Name, "compute")
	}
	callers := cgtest.CollectCallerNames(root)
	for _, want := range []string{"process", "shortcut", "main"} {
		if !callers[want] {
			t.Errorf("expected caller %q in %v", want, callers)
		}
	}
}

func TestMultiFile(t *testing.T) {
	bin := build(t)
	file, _ := filepath.Abs("testdata/multifile/helpers.py")
	// Line 5: `def compute(n):`
	root := cgtest.Run(t, bin, file, 5, 5)
	if root.Name != "compute" {
		t.Fatalf("root name: got %q want %q", root.Name, "compute")
	}
	callers := cgtest.CollectCallerNames(root)
	for _, want := range []string{"process", "also_calls_compute", "main"} {
		if !callers[want] {
			t.Errorf("expected caller %q in %v", want, callers)
		}
	}
}

func TestMultiPkg(t *testing.T) {
	bin := build(t)
	file, _ := filepath.Abs("testdata/multipkg/order/order.py")
	// Line 1: `def create_order():`
	root := cgtest.Run(t, bin, file, 1, 5)
	if root.Name != "create_order" {
		t.Fatalf("root name: got %q want %q", root.Name, "create_order")
	}
	callers := cgtest.CollectCallerNames(root)
	for _, want := range []string{"place_order", "start_router", "main"} {
		if !callers[want] {
			t.Errorf("expected caller %q in %v", want, callers)
		}
	}
}

package rusttests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madhanganesh/callgraph/test/internal/cgtest"
)

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "callgraph-rust-")
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
	cgtest.SkipIfMissing(t, "rust-analyzer")
	cgtest.SkipIfMissing(t, "cargo")
	return binary
}

func TestSimple(t *testing.T) {
	bin := build(t)
	file, _ := filepath.Abs("testdata/simple/src/main.rs")
	// Line 5: `fn compute(n: i32) -> i32 {`
	root := cgtest.Run(t, bin, file, 5, 4)
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
	file, _ := filepath.Abs("testdata/multifile/src/helpers.rs")
	// Line 5: `pub fn compute(n: i32) -> i32 {`
	root := cgtest.Run(t, bin, file, 5, 8)
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
	file, _ := filepath.Abs("testdata/multipkg/crates/order/src/lib.rs")
	// Line 1: `pub fn create_order() {`
	root := cgtest.Run(t, bin, file, 1, 8)
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

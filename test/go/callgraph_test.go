package gotests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/madhanganesh/callgraph/test/internal/cgtest"
)

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "callgraph-go-")
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
	cgtest.SkipIfMissing(t, "gopls")
	return binary
}

// Simple: single file, multiple functions, two callers for compute.
func TestSimple(t *testing.T) {
	bin := build(t)
	file, _ := filepath.Abs("testdata/simple/main.go")
	// Line 21: `func compute(n int) int {`
	root := cgtest.Run(t, bin, file, 21, 6)

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

// MultiFile: caller and callee live in different files of the same package.
func TestMultiFile(t *testing.T) {
	bin := build(t)
	file, _ := filepath.Abs("testdata/multifile/helpers.go")
	// Line 3: `func compute(n int) int {`
	root := cgtest.Run(t, bin, file, 3, 6)

	if root.Name != "compute" {
		t.Fatalf("root name: got %q want %q", root.Name, "compute")
	}
	callers := cgtest.CollectCallerNames(root)
	for _, want := range []string{"process", "alsoCallsCompute", "main"} {
		if !callers[want] {
			t.Errorf("expected caller %q in %v", want, callers)
		}
	}
}

// MultiPkg: callers span multiple packages (main -> api -> v1 -> order).
func TestMultiPkg(t *testing.T) {
	bin := build(t)
	file, _ := filepath.Abs("testdata/multipkg/order/order.go")
	// Line 9: `func CreateOrder() {`
	root := cgtest.Run(t, bin, file, 9, 6)

	if root.Name != "CreateOrder" {
		t.Fatalf("root name: got %q want %q", root.Name, "CreateOrder")
	}
	callers := cgtest.CollectCallerNames(root)
	for _, want := range []string{"PlaceOrder", "AddOrderRoutes", "StartRouter", "main"} {
		if !callers[want] {
			t.Errorf("expected caller %q in %v", want, callers)
		}
	}
}

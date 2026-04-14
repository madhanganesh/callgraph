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

// Callees: outgoing call tree from main should reach process -> compute -> helper.
func TestCallees(t *testing.T) {
	bin := build(t)
	file, _ := filepath.Abs("testdata/simple/main.go")
	// Line 5: `func main() {`
	root := cgtest.RunCallees(t, bin, file, 5, 6)
	if root.Name != "main" {
		t.Fatalf("root name: got %q want %q", root.Name, "main")
	}

	names := map[string]bool{}
	var walk func(*cgtest.Node)
	walk = func(n *cgtest.Node) {
		for _, c := range n.Callees {
			names[c.Name] = true
			walk(c)
		}
	}
	walk(root)

	for _, want := range []string{"process", "shortcut", "compute", "helper"} {
		if !names[want] {
			t.Errorf("expected callee %q in %v", want, names)
		}
	}
}

// Classified: API / DB / thread calls get the right Kind.
func TestClassified(t *testing.T) {
	bin := build(t)
	file, _ := filepath.Abs("testdata/classified/main.go")
	// Line 8: `func handle(db *sql.DB) {`
	root := cgtest.RunCallees(t, bin, file, 8, 6)
	if root.Name != "handle" {
		t.Fatalf("root name: got %q want %q", root.Name, "handle")
	}

	direct := cgtest.CalleesByName(root)
	if got := direct["Get"]; got == nil || got.Kind != cgtest.KindAPI {
		t.Errorf("Get: want Kind=API, got %+v", got)
	}
	if got := direct["Exec"]; got == nil || got.Kind != cgtest.KindDB {
		t.Errorf("Exec: want Kind=DB, got %+v", got)
	} else if got.Detail != "UPDATE t" {
		t.Errorf("Exec: want Detail=%q, got %q", "UPDATE t", got.Detail)
	}
	if got := direct["worker"]; got == nil || got.Kind != cgtest.KindThread {
		t.Errorf("worker: want Kind=Thread, got %+v", got)
	}
}

// Interface: callee mode on an interface method call returns implementations.
func TestInterfacePicker(t *testing.T) {
	bin := build(t)
	file, _ := filepath.Abs("testdata/iface/main.go")
	// Line 22, col 5: cursor on `Save` in `st.Save("hi")`.
	root := cgtest.RunCallees(t, bin, file, 22, 5)

	if len(root.Implementations) < 2 {
		t.Fatalf("want >=2 implementations, got %d (callees=%d)",
			len(root.Implementations), len(root.Callees))
	}
	lines := map[int]bool{}
	for _, impl := range root.Implementations {
		lines[impl.Line] = true
	}
	if len(lines) < 2 {
		t.Errorf("want impls at distinct lines, got %v", lines)
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

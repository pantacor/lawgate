package gocode

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, root, name, text string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

const billing = `package billing

var machine = Machine{Name: "billing"}

var sites = []Site{
	{
		Kind: "fault",
		Tag:  "1",
	},
	{Tag: "2", Match: "retry()"},
}
`

const shipping = `package shipping

var machine = Machine{Name: "shipping"}

var sites = []Site{{Tag: "1"}}
`

// TestFindTellsMachinesApartAndPlacesTheTag: two machines reuse tag "1"; the
// one whose package names the machine wins, and the site sits on its Tag.
func TestFindTellsMachinesApartAndPlacesTheTag(t *testing.T) {
	root := t.TempDir()
	write(t, root, "billing/machine.go", billing)
	write(t, root, "shipping/machine.go", shipping)
	write(t, root, "nested/go.mod", "module nested\n")
	write(t, root, "nested/machine.go", shipping)
	write(t, root, "billing/machine_test.go", shipping)
	literals := ScanLiterals(root)
	if len(literals.Sites) != 3 {
		t.Fatalf("sites = %+v, want the two packages' three (no test file, no nested module)", literals.Sites)
	}
	billed, found := literals.Find("billing", "1", "")
	placed := billed.File == "billing/machine.go" && billed.Line == 8
	if !found || !placed {
		t.Fatalf("billing tag 1 = %+v, want billing/machine.go:8", billed)
	}
	shipped, _ := literals.Find("shipping", "1", "")
	if shipped.File != "shipping/machine.go" {
		t.Fatalf("shipping tag 1 = %+v", shipped)
	}
	matched, _ := literals.Find("billing", "2", "retry()")
	if matched.Line != 10 {
		t.Fatalf("billing tag 2 = %+v", matched)
	}
	if _, found := literals.Find("billing", "9", ""); found {
		t.Fatal("an undeclared tag was found")
	}
}

const methods = `package shop

type cart struct{}

func (c *cart) Total() int {
	return 0
}

func Pay() {
	pay := func() {}
	pay()
}
`

func TestFunctionsAndEnclosing(t *testing.T) {
	root := t.TempDir()
	write(t, root, "shop/cart.go", methods)
	functions := Functions(root, "shop")
	total, found := functions["cart.Total"]
	spans := total.Start == 5 && total.End == 7
	if !found || !spans {
		t.Fatalf("cart.Total = %+v", total)
	}
	inner, found := Enclosing(functions, "shop/cart.go", 10)
	if !found || inner.Start != 9 {
		t.Fatalf("enclosing line 10 = %+v", inner)
	}
	if _, found := FunctionAt(root, "shop/cart.go", 9); !found {
		t.Fatal("Pay is not found at its line")
	}
}

package gen

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pantacor/lawgate/machine"
)

// edge writes the method for one declared edge and propagates drift.
func (w *writer) edge(from, typ string, edge machine.Edge) {
	to := typeName(w.opts.Prefix, edge.To)
	if !w.generated[edge.To] {
		w.outside[edge.To] = true
	}
	w.edgeComment(edge)
	receiver := moveLocal("s", to)
	runRef := receiver + ".run"
	w.printf("func (%s %s) %s() (%s, error) {\n", receiver, typ, edge.Name, to)
	w.checkedMove(to, runRef, "%s.Go(%q, %q, %q)", runRef, from, edge.Name, edge.To)
}

func (w *writer) edgeComment(edge machine.Edge) {
	w.printf("// %s: %s (%s", edge.Name, oneLine(edge.On), edge.Kind)
	if edge.Guard != "" {
		w.printf(", guard: %s", oneLine(edge.Guard))
	}
	if edge.Limit > 0 {
		w.printf(", limit: %d", edge.Limit)
	}
	w.printf(").\n")
}

// checkedMove only creates a typed destination after the run accepts a move.
func (w *writer) checkedMove(to, runRef, format string, args ...any) {
	errName := moveLocal("err", to)
	call := fmt.Sprintf(format, args...)
	w.printf("\tif %s := %s; %s != nil {\n", errName, call, errName)
	w.printf("\t\treturn %s{}, %s\n\t}\n", to, errName)
	w.printf("\treturn %s{run: %s}, nil\n}\n\n", to, runRef)
}

// The destination type is used in the method body, where locals can shadow it.
func moveLocal(name, destinationType string) string {
	if name == destinationType {
		return name + "_"
	}
	return name
}

// typeName turns a state id into a Go type name after prefix: "wait-retry"
// with prefix "app" gives "appWaitRetry".
func typeName(prefix, id string) string {
	var b strings.Builder
	b.WriteString(prefix)
	upper := prefix != ""
	for _, r := range id {
		if isSeparator(r) {
			upper = true
			continue
		}
		if upper {
			r = unicode.ToUpper(r)
		}
		upper = false
		b.WriteRune(r)
	}
	return b.String()
}

func isSeparator(r rune) bool { return strings.ContainsRune("-_ ", r) }

// enterName keeps an Enter function exported exactly when its type is.
func enterName(typ string) string {
	first, width := utf8.DecodeRuneInString(typ)
	if unicode.IsUpper(first) {
		return "Enter" + typ
	}
	return "enter" + string(unicode.ToUpper(first)) + typ[width:]
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

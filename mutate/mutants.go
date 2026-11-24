package mutate

import (
	"go/ast"
	"go/token"
	"slices"
	"strings"
)

// MaxMutants bounds the mutants checked per subject; the rest are counted,
// so a truncated run says how much it left unchecked.
const MaxMutants = 64

// Mutant is the subject's file with one guard condition changed.
type Mutant struct {
	Line        int
	Description string
	Source      []byte
}

// edit replaces source[start:end] with text.
type edit struct {
	start, end  int
	text        string
	line        int
	description string
}

// flips are the comparison mutations: each operator and its boundary twin.
var flips = map[token.Token]token.Token{
	token.LSS: token.LEQ, token.LEQ: token.LSS,
	token.GTR: token.GEQ, token.GEQ: token.GTR,
	token.EQL: token.NEQ, token.NEQ: token.EQL,
}

// Mutants lists the subject's mutants in source order, at most limit of
// them, and how many distinct mutants there were in all.
func (target Target) Mutants(limit int) ([]Mutant, int) {
	collector := target.collect()
	var mutants []Mutant
	seen := map[string]bool{}
	for _, change := range collector.edits {
		mutant := target.apply(change)
		if seen[string(mutant.Source)] {
			continue
		}
		seen[string(mutant.Source)] = true
		mutants = append(mutants, mutant)
	}
	total := len(mutants)
	return mutants[:min(total, limit)], total
}

// ErrorPropagations counts the guards left unmutated because they only pass
// an error up (see the package documentation).
func (target Target) ErrorPropagations() int {
	return target.collect().propagations
}

func (target Target) collect() *editCollector {
	collector := &editCollector{target: target}
	ast.Inspect(target.decl.Body, collector.visit)
	return collector
}

func (target Target) apply(change edit) Mutant {
	source := slices.Concat(target.Source[:change.start], []byte(change.text), target.Source[change.end:])
	return Mutant{Line: change.line, Description: change.description, Source: source}
}

type editCollector struct {
	target       Target
	edits        []edit
	propagations int
}

// visit finds the guards: if, for and expression-less switch conditions,
// the named conditions the readability rule asks for (a variable assigned
// or declared with a condition) and the conditions a function returns.
func (collector *editCollector) visit(node ast.Node) bool {
	switch typed := node.(type) {
	case *ast.AssignStmt:
		collector.namedCondition(typed)
	case *ast.ValueSpec:
		collector.declaredCondition(typed)
	case *ast.ReturnStmt:
		collector.returnedConditions(typed)
	case *ast.IfStmt:
		collector.ifGuard(typed)
	case *ast.ForStmt:
		collector.guard(typed.Cond)
	case *ast.SwitchStmt:
		collector.switchGuards(typed)
	}
	return true
}

func (collector *editCollector) namedCondition(assignment *ast.AssignStmt) {
	single := len(assignment.Lhs) == 1 && len(assignment.Rhs) == 1
	if single && isCondition(assignment.Rhs[0]) {
		collector.guard(assignment.Rhs[0])
	}
}

func (collector *editCollector) declaredCondition(spec *ast.ValueSpec) {
	single := len(spec.Names) == 1 && len(spec.Values) == 1
	if single && isCondition(spec.Values[0]) {
		collector.guard(spec.Values[0])
	}
}

func (collector *editCollector) returnedConditions(statement *ast.ReturnStmt) {
	for _, result := range statement.Results {
		if isCondition(result) {
			collector.guard(result)
		}
	}
}

// ifGuard mutates an if, unless it only propagates an error.
func (collector *editCollector) ifGuard(statement *ast.IfStmt) {
	if propagatesError(statement) {
		collector.propagations++
		return
	}
	collector.guard(statement.Cond)
	collector.disableReturn(statement)
}

// isCondition reports whether the expression is boolean by its form: a
// comparison, a && / || expression or a negation.
func isCondition(expr ast.Expr) bool {
	expr = ast.Unparen(expr)
	if unary, ok := expr.(*ast.UnaryExpr); ok {
		return unary.Op == token.NOT
	}
	binary, ok := expr.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	_, comparison := flips[binary.Op]
	logical := binary.Op == token.LAND || binary.Op == token.LOR
	return comparison || logical
}

func (collector *editCollector) switchGuards(statement *ast.SwitchStmt) {
	if statement.Tag != nil {
		return
	}
	for _, clause := range statement.Body.List {
		for _, condition := range clause.(*ast.CaseClause).List {
			collector.guard(condition)
		}
	}
}

// guard negates the condition and flips each comparison inside it.
func (collector *editCollector) guard(condition ast.Expr) {
	if condition == nil {
		return
	}
	text := collector.text(condition)
	negated := "!(" + text + ")"
	collector.add(condition, negated, "negated the condition "+quote(text))
	ast.Inspect(condition, func(node ast.Node) bool {
		if binary, ok := node.(*ast.BinaryExpr); ok {
			collector.flip(binary, text)
		}
		return true
	})
}

func (collector *editCollector) flip(binary *ast.BinaryExpr, condition string) {
	replacement, ok := flips[binary.Op]
	if !ok {
		return
	}
	start := collector.offset(binary.OpPos)
	change := edit{start: start, end: start + len(binary.Op.String()), text: replacement.String()}
	change.line = collector.target.fset.Position(binary.OpPos).Line
	change.description = "changed " + binary.Op.String() + " to " + replacement.String() + " in " + quote(condition)
	collector.edits = append(collector.edits, change)
}

// disableReturn makes a guarded return unreachable. The condition stays in
// the code, after false &&, so a variable it alone uses still compiles.
func (collector *editCollector) disableReturn(statement *ast.IfStmt) {
	if !returnsGuardValue(statement.Body) {
		return
	}
	text := collector.text(statement.Cond)
	disabled := "false && (" + text + ")"
	collector.add(statement.Cond, disabled, "replaced the condition "+quote(text)+" of a guarded return by false")
}

// returnsGuardValue reports whether the block returns something other than
// nil last: an error, a refusal, a sentinel.
func returnsGuardValue(body *ast.BlockStmt) bool {
	return slices.ContainsFunc(body.List, func(statement ast.Stmt) bool {
		ret, ok := statement.(*ast.ReturnStmt)
		if !ok || len(ret.Results) == 0 {
			return false
		}
		last, isIdent := ret.Results[len(ret.Results)-1].(*ast.Ident)
		return !isIdent || last.Name != "nil"
	})
}

func (collector *editCollector) add(node ast.Node, text, description string) {
	change := edit{start: collector.offset(node.Pos()), end: collector.offset(node.End()), text: text}
	change.line = collector.target.fset.Position(node.Pos()).Line
	change.description = description
	collector.edits = append(collector.edits, change)
}

func (collector *editCollector) offset(pos token.Pos) int {
	return collector.target.fset.Position(pos).Offset
}

func (collector *editCollector) text(node ast.Node) string {
	return string(collector.target.Source[collector.offset(node.Pos()):collector.offset(node.End())])
}

// quote shows a condition on one line, shortened for a verdict.
func quote(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	const limit = 80
	if len(text) > limit {
		text = text[:limit] + "..."
	}
	return "`" + text + "`"
}

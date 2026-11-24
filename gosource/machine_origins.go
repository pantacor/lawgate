package gosource

import (
	"go/ast"
	"go/token"
	"go/types"
)

func (facts *machineFacts) declarations(fset *token.FileSet) []Declaration {
	var declarations []Declaration
	for _, name := range facts.names {
		declarations = append(declarations, Declaration{Name: name.name, Position: fset.Position(name.pos)})
	}
	for _, value := range facts.candidates {
		if literal, ok := value.syntax.(*ast.CompositeLit); ok {
			name := literalMachineName(value.info, literal)
			declarations = append(declarations, Declaration{Name: name, Position: fset.Position(value.pos)})
			continue
		}
		visiting := make(map[types.Object]bool)
		if !facts.knownOrigin(value, visiting) {
			declarations = append(declarations, Declaration{Position: fset.Position(value.pos)})
		}
	}
	return declarations
}

// literalMachineName is the name a machine literal establishes. A literal
// whose type go/types cannot see, such as one of a cgo type, names nothing.
func literalMachineName(info *types.Info, literal *ast.CompositeLit) string {
	literalType := info.TypeOf(literal)
	if literalType == nil {
		return ""
	}
	shape, _ := literalType.Underlying().(*types.Struct)
	for index, element := range literal.Elts {
		expr := nameFieldValue(shape, index, element)
		if expr != nil {
			return stringValue(info, expr)
		}
	}
	return ""
}

func nameFieldValue(shape *types.Struct, index int, expr ast.Expr) ast.Expr {
	if field, ok := expr.(*ast.KeyValueExpr); ok {
		if key, ok := field.Key.(*ast.Ident); ok && key.Name == "Name" {
			return field.Value
		}
		return nil
	}
	validIndex := shape != nil && index < shape.NumFields()
	if validIndex && shape.Field(index).Name() == "Name" {
		return expr
	}
	return nil
}

// Forwarding a known constructor or value introduces no new name. Unknown
// origins stay unresolved, even if a different assignment has a known name.
func (facts *machineFacts) knownOrigin(value machineValue, visiting map[types.Object]bool) bool {
	if value.syntax == nil {
		return false
	}
	switch expr := ast.Unparen(value.syntax).(type) {
	case *ast.CompositeLit:
		return literalMachineName(value.info, expr) != ""
	case *ast.UnaryExpr:
		if expr.Op == token.AND {
			value.syntax = expr.X
			return facts.knownOrigin(value, visiting)
		}
	case *ast.StarExpr:
		value.syntax = expr.X
		return facts.knownOrigin(value, visiting)
	case *ast.CallExpr:
		return facts.knownCall(expr, value, visiting)
	case *ast.Ident, *ast.SelectorExpr:
		object := expressionObject(value.info, expr)
		field := receiverField(value.info, expr)
		return facts.fieldNameKnown(field, value) || facts.knownVariable(field, object, value.pos, visiting)
	}
	return false
}

func expressionObject(info *types.Info, expr ast.Expr) types.Object {
	switch name := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return info.ObjectOf(name)
	case *ast.SelectorExpr:
		return info.Uses[name.Sel]
	}
	return nil
}

func (facts *machineFacts) knownCall(call *ast.CallExpr, value machineValue, visiting map[types.Object]bool) bool {
	if _, literal := ast.Unparen(call.Fun).(*ast.FuncLit); literal {
		return true // The literal's own return statements are checked separately.
	}
	conversion := conversionTarget(value.info, call) != nil
	if conversion {
		value.syntax = call.Args[0]
		return facts.knownOrigin(value, visiting)
	}
	return facts.functions[expressionObject(value.info, call.Fun)]
}

// Function inputs carry origins from the call sites. The body may forward
// an input, but an assignment to it must establish its own origin.
func (facts *machineFacts) forwardedInput(field machineField, pos token.Pos) bool {
	return facts.inputs[field.object] && !facts.fieldReplaced(field, token.NoPos, pos)
}

func (facts *machineFacts) knownVariable(field machineField, object types.Object, pos token.Pos, visiting map[types.Object]bool) bool {
	if facts.forwardedInput(field, pos) {
		return true
	}
	values := facts.origins[object]
	if len(values) == 0 || visiting[object] {
		return false
	}
	visiting[object] = true
	defer delete(visiting, object)
	for _, value := range values {
		if !facts.knownOrigin(value, visiting) {
			return false
		}
	}
	return true
}

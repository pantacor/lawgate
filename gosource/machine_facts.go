package gosource

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
)

type machineFacts struct {
	names      []machineName
	candidates []machineValue
	origins    map[types.Object][]machineValue
	functions  map[types.Object]bool
	inputs     map[types.Object]bool
	fieldNames map[machineField][]nameWrite
	writes     map[types.Object][]fieldWrite
}

type machineScan struct {
	*machineFacts
	info     *types.Info
	machines []types.Type
}

type machineName struct {
	name string
	pos  token.Pos
}

type machineValue struct {
	syntax ast.Expr
	info   *types.Info
	pos    token.Pos
	block  *ast.BlockStmt
}

func newMachineFacts() *machineFacts {
	return &machineFacts{
		origins:    make(map[types.Object][]machineValue),
		functions:  make(map[types.Object]bool),
		inputs:     make(map[types.Object]bool),
		fieldNames: make(map[machineField][]nameWrite),
		writes:     make(map[types.Object][]fieldWrite),
	}
}

func (facts *machineFacts) collect(pkg *typedSource, machines []types.Type) {
	scan := machineScan{machineFacts: facts, info: pkg.info, machines: machines}
	for _, file := range pkg.files {
		ast.PreorderStack(file, nil, scan.visit)
	}
}

func (scan machineScan) visit(node ast.Node, parents []ast.Node) bool {
	switch item := node.(type) {
	case *ast.FuncDecl:
		if object := scan.info.Defs[item.Name]; object != nil {
			scan.functions[object] = item.Body != nil
		}
		scan.parameters(item.Type.Params)
		scan.parameters(item.Recv)
	case *ast.FuncLit:
		scan.parameters(item.Type.Params)
	case *ast.CompositeLit:
		scan.composite(item, parents)
	case *ast.CallExpr:
		scan.call(item, parents)
	case *ast.SendStmt:
		scan.publish(item.Value, parents)
	case *ast.ValueSpec:
		scan.variableDeclarations(item, parents)
	case *ast.AssignStmt:
		scan.assignments(item, parents)
	case *ast.ReturnStmt:
		scan.returns(item, parents)
	}
	return true
}

func (scan machineScan) parameters(fields *ast.FieldList) {
	for _, expr := range resultNames(fields) {
		if object := scan.info.ObjectOf(expr.(*ast.Ident)); object != nil {
			scan.inputs[object] = true
		}
	}
}

func (scan machineScan) isMachine(expr ast.Expr) bool {
	return isMachine(scan.valueType(expr), scan.machines)
}

func (scan machineScan) value(expr ast.Expr, pos token.Pos) machineValue {
	return machineValue{syntax: expr, info: scan.info, pos: pos}
}

func (scan machineScan) candidate(expr ast.Expr, pos token.Pos) {
	scan.candidates = append(scan.candidates, scan.value(expr, pos))
}

func (scan machineScan) composite(literal *ast.CompositeLit, parents []ast.Node) {
	if scan.isMachine(literal) {
		named := literalMachineName(scan.info, literal) != ""
		if named || !scan.boundLocally(literal, parents) {
			scan.candidate(literal, literal.Pos())
		}
	}
	for _, element := range literal.Elts {
		if entry, keyed := element.(*ast.KeyValueExpr); keyed {
			scan.publish(entry.Key, parents)
			element = entry.Value
		}
		scan.publish(element, parents)
	}
}

func (scan machineScan) call(call *ast.CallExpr, parents []ast.Node) {
	if scan.isMachine(call) && isNew(call, scan.info) {
		if !scan.boundLocally(call, parents) {
			scan.candidate(call, call.Pos())
		}
		return
	}
	// A conversion to a concrete type keeps its value to itself; one to an
	// interface type hands it on like an argument. The target decides, since
	// go/types withholds the result type of a conversion of a cgo operand.
	target := conversionTarget(scan.info, call)
	concrete := target != nil && !types.IsInterface(target)
	if concrete {
		return
	}
	for _, arg := range call.Args {
		scan.publish(arg, parents)
	}
}

func (scan machineScan) publish(expr ast.Expr, parents []ast.Node) {
	if !scan.isMachine(expr) {
		return
	}
	value := scan.value(expr, expr.Pos())
	value.block = publicationBlock(parents)
	scan.candidates = append(scan.candidates, value)
}

// A direct local binding can establish its name before publishing the value.
func (scan machineScan) boundLocally(expr ast.Expr, parents []ast.Node) bool {
	for i := len(parents) - 1; i >= 0; i-- {
		switch node := parents[i].(type) {
		case *ast.ParenExpr, *ast.UnaryExpr:
			expr = node.(ast.Expr)
		case *ast.ValueSpec:
			return scan.localBinding(node.Values, identifiers(node.Names), expr)
		case *ast.AssignStmt:
			return scan.localBinding(node.Rhs, node.Lhs, expr)
		default:
			return false
		}
	}
	return false
}

func (scan machineScan) localBinding(values, targets []ast.Expr, expr ast.Expr) bool {
	for i, value := range values {
		if value != expr || i >= len(targets) {
			continue
		}
		name, _ := ast.Unparen(targets[i]).(*ast.Ident)
		object := scan.info.ObjectOf(name)
		return localVariable(object) && isMachine(object.Type(), scan.machines)
	}
	return false
}

func identifiers(names []*ast.Ident) []ast.Expr {
	values := make([]ast.Expr, len(names))
	for i, name := range names {
		values[i] = name
	}
	return values
}

func localVariable(object types.Object) bool {
	if object == nil || object.Name() == "_" {
		return false
	}
	return object.Parent() != object.Pkg().Scope()
}

func (scan machineScan) variableDeclarations(spec *ast.ValueSpec, parents []ast.Node) {
	for i, name := range spec.Names {
		object := scan.info.Defs[name]
		value := expressionAt(spec.Values, i, len(spec.Names))
		if !scan.bindVariable(object, value, name.Pos(), parents) {
			scan.publish(value, parents)
		}
	}
}

func (scan machineScan) bindVariable(object types.Object, rhs ast.Expr, pos token.Pos, parents []ast.Node) bool {
	if object == nil || object.Name() == "_" {
		return false
	}
	knownType := isMachine(object.Type(), scan.machines)
	if !knownType {
		return false
	}
	value := scan.value(rhs, pos)
	value.block = bindingBlock(parents)
	scan.origins[object] = append(scan.origins[object], value)
	if !localVariable(object) {
		scan.candidate(rhs, pos)
	}
	return true
}

func expressionAt(expressions []ast.Expr, index, count int) ast.Expr {
	if len(expressions) == count {
		return expressions[index]
	}
	if len(expressions) == 1 {
		return expressions[0]
	}
	return nil
}

func (scan machineScan) assignments(stmt *ast.AssignStmt, parents []ast.Node) {
	for i, lhs := range stmt.Lhs {
		rhs := expressionAt(stmt.Rhs, i, len(stmt.Lhs))
		scan.assign(ast.Unparen(lhs), rhs, stmt.Tok, parents)
	}
}

func (scan machineScan) assign(lhs, rhs ast.Expr, op token.Token, parents []ast.Node) {
	field := receiverField(scan.info, lhs)
	if field.object != nil {
		write := fieldWrite{fields: field.fields, pos: lhs.Pos()}
		scan.writes[field.object] = append(scan.writes[field.object], write)
	}
	switch dest := lhs.(type) {
	case *ast.Ident:
		if scan.bindVariable(scan.info.ObjectOf(dest), rhs, dest.Pos(), parents) {
			return
		}
	case *ast.SelectorExpr:
		scan.nameAssignment(dest, rhs, op, parents)
	}
	scan.publish(rhs, parents)
}

func (scan machineScan) nameAssignment(selector *ast.SelectorExpr, rhs ast.Expr, operation token.Token, parents []ast.Node) {
	field, namedField := scan.nameReceiver(selector)
	if !namedField {
		return
	}
	var name string
	if operation == token.ASSIGN {
		name = stringValue(scan.info, rhs)
	}
	scan.names = append(scan.names, machineName{name: name, pos: selector.Pos()})
	if field.object != nil {
		write := nameWrite{name: name, pos: selector.Pos(), block: statementBlock(parents)}
		scan.fieldNames[field] = append(scan.fieldNames[field], write)
	}
}

func (scan machineScan) returns(stmt *ast.ReturnStmt, parents []ast.Node) {
	exprs := stmt.Results
	if len(exprs) == 0 {
		exprs = namedResults(parents)
	}
	for _, expr := range exprs {
		if returnsMachine(scan.valueType(expr), scan.machines) {
			value := scan.value(expr, stmt.Pos())
			value.block = straightLineBlock(parents)
			scan.candidates = append(scan.candidates, value)
		}
	}
}

func namedResults(parents []ast.Node) []ast.Expr {
	for i := len(parents) - 1; i >= 0; i-- {
		switch fn := parents[i].(type) {
		case *ast.FuncDecl:
			return resultNames(fn.Type.Results)
		case *ast.FuncLit:
			return resultNames(fn.Type.Results)
		}
	}
	return nil
}

func resultNames(results *ast.FieldList) []ast.Expr {
	if results == nil {
		return nil
	}
	var names []ast.Expr
	for _, field := range results.List {
		for _, name := range field.Names {
			names = append(names, name)
		}
	}
	return names
}

func returnsMachine(t types.Type, machines []types.Type) bool {
	if tuple, ok := t.(*types.Tuple); ok {
		for i := 0; i < tuple.Len(); i++ {
			if isMachine(tuple.At(i).Type(), machines) {
				return true
			}
		}
		return false
	}
	return isMachine(t, machines)
}

// valueType is the type of expr when expr is a value go/types knows the
// type of, else nil. A value whose type it cannot know, such as a cgo
// call, is no machine and publishes nothing. A conversion of such a value
// records no result, yet it still converts to the type it names.
func (scan machineScan) valueType(expr ast.Expr) types.Type {
	recorded := scan.info.Types[expr]
	if recorded.IsValue() {
		return recorded.Type
	}
	call, isCall := expr.(*ast.CallExpr)
	if !isCall {
		return nil
	}
	return conversionTarget(scan.info, call)
}

// conversionTarget is the type a call converts its argument to, or nil
// when the call is not a conversion.
func conversionTarget(info *types.Info, call *ast.CallExpr) types.Type {
	target := info.Types[call.Fun]
	if !target.IsType() || len(call.Args) != 1 {
		return nil
	}
	return target.Type
}

func isNew(call *ast.CallExpr, info *types.Info) bool {
	id, ok := ast.Unparen(call.Fun).(*ast.Ident)
	return ok && info.Uses[id] == types.Universe.Lookup("new")
}

func stringValue(info *types.Info, expr ast.Expr) string {
	value := info.Types[expr].Value
	if value != nil && value.Kind() == constant.String {
		return constant.StringVal(value)
	}
	return ""
}

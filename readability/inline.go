package readability

import (
	"fmt"
	"go/ast"
	"go/token"
)

type inlineChecker struct {
	fset         *token.FileSet
	path         string
	arrayAliases map[string]*ast.ArrayType
	mapAliases   map[string]*ast.MapType
	findings     []Finding
}

// inlineScope tracks the enclosing object (struct/map) and enclosing sequence
// (slice/array) composite literals for the current expression.
type inlineScope struct {
	outer    *ast.CompositeLit
	outerSeq *ast.CompositeLit
}

func checkInlineNesting(fset *token.FileSet, path string, file *ast.File) []Finding {
	arrays, maps := collectTypeAliases(file)
	c := &inlineChecker{
		fset:         fset,
		path:         path,
		arrayAliases: arrays,
		mapAliases:   maps,
	}
	for _, decl := range file.Decls {
		c.checkDecl(decl)
	}
	return c.findings
}

func collectTypeAliases(file *ast.File) (map[string]*ast.ArrayType, map[string]*ast.MapType) {
	arrays := make(map[string]*ast.ArrayType)
	maps := make(map[string]*ast.MapType)
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		recordTypeSpecs(gd.Specs, arrays, maps)
	}
	return arrays, maps
}

func recordTypeSpecs(
	specs []ast.Spec,
	arrays map[string]*ast.ArrayType,
	maps map[string]*ast.MapType,
) {
	for _, spec := range specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		switch t := ts.Type.(type) {
		case *ast.ArrayType:
			arrays[ts.Name.Name] = t
		case *ast.MapType:
			maps[ts.Name.Name] = t
		}
	}
}

func (c *inlineChecker) checkDecl(decl ast.Decl) {
	if fd, ok := decl.(*ast.FuncDecl); ok {
		if fd.Body != nil {
			c.walkNode(fd.Body, funcDeclName(fd))
		}
		return
	}
	if gd, ok := decl.(*ast.GenDecl); ok {
		c.checkGenDecl(gd)
	}
}

func (c *inlineChecker) checkGenDecl(gd *ast.GenDecl) {
	for _, spec := range gd.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, val := range vs.Values {
			c.walkNode(val, "")
		}
	}
}

func (c *inlineChecker) walkNode(node ast.Node, fnName string) {
	if node == nil {
		return
	}
	ast.Inspect(node, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok {
			c.walkNode(lit.Body, funcLitName(fnName))
			return false
		}
		if comp, ok := n.(*ast.CompositeLit); ok {
			c.checkCompositeLit(comp, fnName, inlineScope{}, nil)
			return false
		}
		return true
	})
}

func (c *inlineChecker) checkCompositeLit(
	lit *ast.CompositeLit,
	fnName string,
	scope inlineScope,
	expected ast.Expr,
) {
	effType := effectiveLitType(lit.Type, expected)
	if arr := c.resolveArrayType(effType); arr != nil {
		if scope.outerSeq != nil {
			c.report(scope.outerSeq, lit, fnName)
		}
		next := inlineScope{outer: scope.outer, outerSeq: lit}
		c.checkSliceElts(lit.Elts, fnName, next, arr.Elt)
		return
	}
	if scope.outer != nil {
		c.report(scope.outer, lit, fnName)
	}
	next := inlineScope{outer: lit}
	if mt := c.resolveMapType(effType); mt != nil {
		c.checkMapElts(lit.Elts, fnName, next, mt)
		return
	}
	c.checkStructElts(lit.Elts, fnName, next)
}

func (c *inlineChecker) checkSliceElts(
	elts []ast.Expr,
	fnName string,
	scope inlineScope,
	eltType ast.Expr,
) {
	for _, elt := range elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			c.walkNode(kv.Key, fnName)
			c.checkExpr(kv.Value, fnName, scope, eltType)
			continue
		}
		c.checkExpr(elt, fnName, scope, eltType)
	}
}

func (c *inlineChecker) checkMapElts(
	elts []ast.Expr,
	fnName string,
	scope inlineScope,
	mt *ast.MapType,
) {
	for _, elt := range elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			c.checkExpr(kv.Key, fnName, scope, mt.Key)
			c.checkExpr(kv.Value, fnName, scope, mt.Value)
			continue
		}
		c.checkExpr(elt, fnName, scope, nil)
	}
}

func (c *inlineChecker) checkStructElts(
	elts []ast.Expr,
	fnName string,
	scope inlineScope,
) {
	for _, elt := range elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			c.checkExpr(kv.Value, fnName, scope, nil)
			continue
		}
		c.checkExpr(elt, fnName, scope, nil)
	}
}

func (c *inlineChecker) checkExpr(
	expr ast.Expr,
	fnName string,
	scope inlineScope,
	expected ast.Expr,
) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.ParenExpr:
		c.checkExpr(e.X, fnName, scope, expected)
	case *ast.UnaryExpr:
		c.checkUnaryExpr(e, fnName, scope, expected)
	case *ast.CompositeLit:
		c.checkCompositeLit(e, fnName, scope, expected)
	case *ast.FuncLit:
		c.walkNode(e.Body, funcLitName(fnName))
	default:
		c.walkNode(e, fnName)
	}
}

func (c *inlineChecker) checkUnaryExpr(
	e *ast.UnaryExpr,
	fnName string,
	scope inlineScope,
	expected ast.Expr,
) {
	if e.Op == token.AND {
		c.checkExpr(e.X, fnName, scope, unwrapType(expected))
		return
	}
	c.walkNode(e.X, fnName)
}

func (c *inlineChecker) resolveArrayType(expr ast.Expr) *ast.ArrayType {
	if expr == nil {
		return nil
	}
	if arr, ok := expr.(*ast.ArrayType); ok {
		return arr
	}
	if id, ok := expr.(*ast.Ident); ok {
		return c.arrayAliases[id.Name]
	}
	return nil
}

func (c *inlineChecker) resolveMapType(expr ast.Expr) *ast.MapType {
	if expr == nil {
		return nil
	}
	if mt, ok := expr.(*ast.MapType); ok {
		return mt
	}
	if id, ok := expr.(*ast.Ident); ok {
		return c.mapAliases[id.Name]
	}
	return nil
}

func (c *inlineChecker) report(outer, inner *ast.CompositeLit, fnName string) {
	outerPos := c.fset.Position(outer.Pos())
	innerPos := c.fset.Position(inner.Pos())
	msg := fmt.Sprintf(
		"inline composite literal at %s:%d:%d nested inside composite literal at %s:%d:%d",
		c.path, innerPos.Line, innerPos.Column,
		c.path, outerPos.Line, outerPos.Column,
	)
	finding := Finding{
		Rule:    RuleInlineNesting,
		Path:    c.path,
		Line:    innerPos.Line,
		Func:    fnName,
		Message: msg,
	}
	c.findings = append(c.findings, finding)
}

func effectiveLitType(explicit, expected ast.Expr) ast.Expr {
	if explicit != nil {
		return unwrapType(explicit)
	}
	return unwrapType(expected)
}

func unwrapType(expr ast.Expr) ast.Expr {
	for expr != nil {
		switch t := expr.(type) {
		case *ast.ParenExpr:
			expr = t.X
		case *ast.StarExpr:
			expr = t.X
		default:
			return expr
		}
	}
	return nil
}

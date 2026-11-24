package readability

import (
	"fmt"
	"go/ast"
	"go/token"
)

const maxBoolOperands = 2

type conditionChecker struct {
	fset     *token.FileSet
	path     string
	findings []Finding
}

func checkConditions(fset *token.FileSet, path string, file *ast.File) []Finding {
	c := &conditionChecker{fset: fset, path: path}
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			c.walkNode(decl, "")
			continue
		}
		if fd.Body == nil {
			continue
		}
		c.walkNode(fd.Body, funcDeclName(fd))
	}
	return c.findings
}

func (c *conditionChecker) walkNode(node ast.Node, fnName string) {
	if node == nil {
		return
	}
	ast.Inspect(node, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok {
			c.walkNode(lit.Body, funcLitName(fnName))
			return false
		}
		c.checkNode(n, fnName)
		return true
	})
}

func (c *conditionChecker) checkNode(n ast.Node, fnName string) {
	switch s := n.(type) {
	case *ast.IfStmt:
		c.checkBoolExpr("if", s.Cond, fnName)
	case *ast.ForStmt:
		c.checkBoolExpr("for", s.Cond, fnName)
	case *ast.SwitchStmt:
		if s.Tag == nil {
			c.checkSwitchCases(s.Body, fnName)
		}
	case *ast.ReturnStmt:
		for _, res := range s.Results {
			c.checkBoolExpr("return", res, fnName)
		}
	}
}

func (c *conditionChecker) checkSwitchCases(body *ast.BlockStmt, fnName string) {
	for _, stmt := range body.List {
		clause := stmt.(*ast.CaseClause)
		for _, expr := range clause.List {
			c.checkBoolExpr("case", expr, fnName)
		}
	}
}

func (c *conditionChecker) checkBoolExpr(kind string, expr ast.Expr, fnName string) {
	if expr == nil {
		return
	}
	count := countBoolOperands(expr)
	if count <= maxBoolOperands {
		return
	}
	line := c.fset.Position(expr.Pos()).Line
	msg := fmt.Sprintf("%s condition has %d boolean operands (max %d)", kind, count, maxBoolOperands)
	finding := Finding{
		Rule:    RuleCondition,
		Path:    c.path,
		Line:    line,
		Func:    fnName,
		Message: msg,
	}
	c.findings = append(c.findings, finding)
}

func countBoolOperands(expr ast.Expr) int {
	for {
		if p, ok := expr.(*ast.ParenExpr); ok {
			expr = p.X
			continue
		}
		if u, ok := expr.(*ast.UnaryExpr); ok && u.Op == token.NOT {
			expr = u.X
			continue
		}
		break
	}
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok {
		return 1
	}
	if bin.Op != token.LAND && bin.Op != token.LOR {
		return 1
	}
	return countBoolOperands(bin.X) + countBoolOperands(bin.Y)
}

package readability

import (
	"fmt"
	"go/ast"
	"go/token"
)

const maxNestingDepth = 2

type blockKind int

const (
	ordinaryBlock blockKind = iota
	loopBlock
)

type nestingChecker struct {
	fset     *token.FileSet
	path     string
	findings []Finding
}

func checkNesting(fset *token.FileSet, path string, file *ast.File) []Finding {
	c := &nestingChecker{fset: fset, path: path}
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			c.scanFuncLits(decl, "", 0)
			continue
		}
		if fd.Body == nil {
			continue
		}
		c.checkBlock(fd.Body, funcDeclName(fd), 0, ordinaryBlock)
	}
	return c.findings
}

// scanFuncLits checks closures inside root starting from the enclosing depth,
// so indentation inside a callback does not hide deep nesting.
func (c *nestingChecker) scanFuncLits(root ast.Node, parent string, depth int) {
	if root == nil {
		return
	}
	ast.Inspect(root, func(n ast.Node) bool {
		lit, ok := n.(*ast.FuncLit)
		if !ok {
			return true
		}
		c.checkBlock(lit.Body, funcLitName(parent), depth, ordinaryBlock)
		return false
	})
}

func (c *nestingChecker) checkBlock(block *ast.BlockStmt, fnName string, depth int, bodyKind blockKind) {
	if block == nil {
		return
	}
	for i, stmt := range block.List {
		statementKind := ordinaryBlock
		if bodyKind == loopBlock && i == 0 {
			statementKind = loopBlock
		}
		c.checkStmt(stmt, fnName, depth, statementKind)
	}
}

func (c *nestingChecker) checkStmt(stmt ast.Stmt, fnName string, depth int, kind blockKind) {
	if lbl, ok := stmt.(*ast.LabeledStmt); ok {
		c.checkStmt(lbl.Stmt, fnName, depth, kind)
		return
	}
	if depth > maxNestingDepth {
		c.report(stmt.Pos(), fnName, depth)
	}
	c.descendStmt(stmt, fnName, depth, kind)
}

func (c *nestingChecker) descendStmt(stmt ast.Stmt, fnName string, depth int, kind blockKind) {
	switch s := stmt.(type) {
	case *ast.IfStmt:
		c.checkIfStmt(s, fnName, depth, kind)
	case *ast.ForStmt:
		c.checkForStmt(s, fnName, depth)
	case *ast.RangeStmt:
		c.checkRangeStmt(s, fnName, depth)
	case *ast.SwitchStmt:
		c.scanNodes(fnName, depth, s.Init, s.Tag)
		c.checkSwitchBody(s.Body, fnName, depth+1)
	case *ast.TypeSwitchStmt:
		c.scanNodes(fnName, depth, s.Init, s.Assign)
		c.checkSwitchBody(s.Body, fnName, depth+1)
	case *ast.SelectStmt:
		c.checkSelectBody(s.Body, fnName, depth+1)
	case *ast.BlockStmt:
		c.checkBlock(s, fnName, depth+1, ordinaryBlock)
	default:
		c.scanFuncLits(stmt, fnName, depth)
	}
}

func (c *nestingChecker) checkForStmt(s *ast.ForStmt, fnName string, depth int) {
	c.scanNodes(fnName, depth, s.Init, s.Cond, s.Post)
	c.checkBlock(s.Body, fnName, depth+1, loopBlock)
}

func (c *nestingChecker) checkRangeStmt(s *ast.RangeStmt, fnName string, depth int) {
	c.scanNodes(fnName, depth, s.Key, s.Value, s.X)
	c.checkBlock(s.Body, fnName, depth+1, loopBlock)
}

func (c *nestingChecker) scanNodes(fnName string, depth int, nodes ...ast.Node) {
	for _, node := range nodes {
		c.scanFuncLits(node, fnName, depth)
	}
}

func (c *nestingChecker) checkIfStmt(s *ast.IfStmt, fnName string, depth int, kind blockKind) {
	c.scanNodes(fnName, depth, s.Init, s.Cond)
	bodyDepth := depth + 1
	if kind == loopBlock && isLoopGuardIf(s) {
		bodyDepth = depth
	}
	c.checkBlock(s.Body, fnName, bodyDepth, ordinaryBlock)
	c.checkElse(s.Else, fnName, depth)
}

func (c *nestingChecker) checkElse(elseStmt ast.Stmt, fnName string, depth int) {
	if elseStmt == nil {
		return
	}
	if elseIf, ok := elseStmt.(*ast.IfStmt); ok {
		c.checkIfStmt(elseIf, fnName, depth, ordinaryBlock)
		return
	}
	if blk, ok := elseStmt.(*ast.BlockStmt); ok {
		c.checkBlock(blk, fnName, depth+1, ordinaryBlock)
	}
}

func (c *nestingChecker) checkSwitchBody(body *ast.BlockStmt, fnName string, depth int) {
	if body == nil {
		return
	}
	for _, stmt := range body.List {
		cc, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		for _, expr := range cc.List {
			c.scanFuncLits(expr, fnName, depth-1)
		}
		c.checkStmtList(cc.Body, fnName, depth)
	}
}

func (c *nestingChecker) checkSelectBody(body *ast.BlockStmt, fnName string, depth int) {
	if body == nil {
		return
	}
	for _, stmt := range body.List {
		cc, ok := stmt.(*ast.CommClause)
		if !ok {
			continue
		}
		c.scanFuncLits(cc.Comm, fnName, depth-1)
		c.checkStmtList(cc.Body, fnName, depth)
	}
}

func (c *nestingChecker) checkStmtList(stmts []ast.Stmt, fnName string, depth int) {
	for _, s := range stmts {
		c.checkStmt(s, fnName, depth, ordinaryBlock)
	}
}

func (c *nestingChecker) report(pos token.Pos, fnName string, depth int) {
	line := c.fset.Position(pos).Line
	msg := fmt.Sprintf("%s:%d: %s nested %d levels deep (max %d)", c.path, line, fnName, depth, maxNestingDepth)
	finding := Finding{
		Rule:    RuleNesting,
		Path:    c.path,
		Line:    line,
		Func:    fnName,
		Message: msg,
	}
	c.findings = append(c.findings, finding)
}

func isLoopGuardIf(s *ast.IfStmt) bool {
	if s.Else != nil || s.Body == nil {
		return false
	}
	if len(s.Body.List) != 1 {
		return false
	}
	return isGuardExitStmt(s.Body.List[0])
}

func isGuardExitStmt(stmt ast.Stmt) bool {
	if _, ok := stmt.(*ast.ReturnStmt); ok {
		return true
	}
	br, ok := stmt.(*ast.BranchStmt)
	if !ok {
		return false
	}
	return br.Tok == token.CONTINUE || br.Tok == token.BREAK
}

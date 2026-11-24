package readability

import (
	"fmt"
	"go/ast"
	"go/token"
)

const maxFuncBodyLines = 40

type lengthChecker struct {
	fset     *token.FileSet
	path     string
	findings []Finding
}

func checkFuncLength(fset *token.FileSet, path string, file *ast.File) []Finding {
	c := &lengthChecker{fset: fset, path: path}
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			c.walkFuncLits(decl, "")
			continue
		}
		if fd.Body == nil {
			continue
		}
		name := funcDeclName(fd)
		c.checkBody(fd.Body, fd.Name.Pos(), name)
		c.walkFuncLits(fd.Body, name)
	}
	return c.findings
}

func (c *lengthChecker) walkFuncLits(node ast.Node, parent string) {
	if node == nil {
		return
	}
	ast.Inspect(node, func(n ast.Node) bool {
		lit, ok := n.(*ast.FuncLit)
		if !ok {
			return true
		}
		name := funcLitName(parent)
		c.checkBody(lit.Body, lit.Pos(), name)
		c.walkFuncLits(lit.Body, name)
		return false
	})
}

func (c *lengthChecker) checkBody(body *ast.BlockStmt, pos token.Pos, name string) {
	if body == nil {
		return
	}
	start := c.fset.Position(body.Lbrace).Line
	end := c.fset.Position(body.Rbrace).Line
	span := end - start + 1
	if span <= maxFuncBodyLines {
		return
	}
	line := c.fset.Position(pos).Line
	msg := fmt.Sprintf("function %s body spans %d lines (max %d)", name, span, maxFuncBodyLines)
	finding := Finding{
		Rule:    RuleFuncLength,
		Path:    c.path,
		Line:    line,
		Func:    name,
		Message: msg,
	}
	c.findings = append(c.findings, finding)
}

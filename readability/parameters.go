package readability

import (
	"go/ast"
	"go/token"
)

func checkBoolParams(positions *token.FileSet, path string, file *ast.File) []Finding {
	var findings []Finding
	ast.Inspect(file, func(node ast.Node) bool {
		switch function := node.(type) {
		case *ast.FuncDecl:
			findings = append(findings, boolParams(positions, path, funcDeclName(function), function.Type.Params)...)
		case *ast.FuncLit:
			findings = append(findings, boolParams(positions, path, funcLitName(""), function.Type.Params)...)
		}
		return true
	})
	return findings
}

func boolParams(positions *token.FileSet, path, name string, params *ast.FieldList) []Finding {
	var findings []Finding
	for _, field := range params.List {
		identifier, ok := field.Type.(*ast.Ident)
		if !ok || identifier.Name != "bool" {
			continue
		}
		finding := Finding{
			Rule: RuleBoolParam, Path: path, Line: positions.Position(field.Pos()).Line,
			Func: name, Message: "function takes a bool parameter",
		}
		findings = append(findings, finding)
	}
	return findings
}

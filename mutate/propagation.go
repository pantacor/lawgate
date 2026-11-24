package mutate

import (
	"go/ast"
	"go/token"
	"slices"
	"strconv"
	"strings"
)

// propagatesError reports whether the if only passes an error up unchanged:
// `if err != nil { return <zero values>, err }`, the error bare, wrapped by
// fmt.Errorf with %w, or joined by errors.Join.
func propagatesError(statement *ast.IfStmt) bool {
	name, isCheck := errorCheck(statement.Cond)
	single := isCheck && statement.Else == nil && len(statement.Body.List) == 1
	if !single {
		return false
	}
	ret, isReturn := statement.Body.List[0].(*ast.ReturnStmt)
	return isReturn && returnsOnly(ret, name)
}

// errorCheck matches `<name> != nil` for a name that holds an error.
func errorCheck(condition ast.Expr) (string, bool) {
	binary, ok := ast.Unparen(condition).(*ast.BinaryExpr)
	if !ok || binary.Op != token.NEQ {
		return "", false
	}
	name, isIdent := binary.X.(*ast.Ident)
	if !isIdent || !isIdentNamed(binary.Y, "nil") {
		return "", false
	}
	return name.Name, strings.HasSuffix(strings.ToLower(name.Name), "err")
}

// returnsOnly reports whether the return gives zero values and then the error.
func returnsOnly(ret *ast.ReturnStmt, name string) bool {
	if len(ret.Results) == 0 {
		return false
	}
	last := len(ret.Results) - 1
	zeros := !slices.ContainsFunc(ret.Results[:last], notZero)
	return zeros && carries(ret.Results[last], name)
}

func notZero(expr ast.Expr) bool {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name != "nil" && typed.Name != "false"
	case *ast.BasicLit:
		return !zeroLiteral(typed)
	case *ast.CompositeLit:
		return len(typed.Elts) != 0
	}
	return true
}

func zeroLiteral(literal *ast.BasicLit) bool {
	if literal.Kind == token.STRING {
		text, err := strconv.Unquote(literal.Value)
		return err == nil && text == ""
	}
	value, err := strconv.ParseFloat(literal.Value, 64)
	return err == nil && value == 0
}

// carries reports whether the expression is the error, wrapped or not.
func carries(expr ast.Expr, name string) bool {
	if isIdentNamed(expr, name) {
		return true
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch callee(call) {
	case "fmt.Errorf":
		return wraps(call, name)
	case "errors.Join":
		return passes(call.Args, name)
	}
	return false
}

// wraps reports whether fmt.Errorf's literal format has %w and the error is
// among its arguments.
func wraps(call *ast.CallExpr, name string) bool {
	if len(call.Args) < 2 {
		return false
	}
	format, isLiteral := call.Args[0].(*ast.BasicLit)
	hasWrap := isLiteral && strings.Contains(format.Value, "%w")
	return hasWrap && passes(call.Args[1:], name)
}

func passes(args []ast.Expr, name string) bool {
	return slices.ContainsFunc(args, func(arg ast.Expr) bool {
		return isIdentNamed(arg, name)
	})
}

// callee is a package-qualified call's name, such as fmt.Errorf.
func callee(call *ast.CallExpr) string {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pkg, isIdent := selector.X.(*ast.Ident)
	if !isIdent {
		return ""
	}
	return pkg.Name + "." + selector.Sel.Name
}

func isIdentNamed(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}

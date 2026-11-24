package gosource

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"
)

type nameWrite struct {
	name  string
	pos   token.Pos
	block *ast.BlockStmt
}

type machineField struct {
	object types.Object
	fields string
}

type fieldWrite struct {
	fields string
	pos    token.Pos
}

func (write fieldWrite) contains(field machineField) bool {
	return write.fields == field.fields || strings.HasPrefix(field.fields, write.fields+"/")
}

func (scan machineScan) nameReceiver(selector *ast.SelectorExpr) (machineField, bool) {
	if selector.Sel.Name != "Name" {
		return machineField{}, false
	}
	selection := scan.info.Selections[selector]
	receiver := scan.info.TypeOf(selector.X)
	if selection == nil || receiver == nil {
		return machineField{}, false
	}
	indices := selection.Index()
	receiverPath := indices[:len(indices)-1]
	receiver = embeddedType(receiver, receiverPath)
	field := receiverField(scan.info, selector.X)
	return field.add(receiverPath), isMachine(receiver, scan.machines)
}

func embeddedType(receiver types.Type, indices []int) types.Type {
	for _, index := range indices {
		shape := receiver.Underlying()
		if pointer, ok := shape.(*types.Pointer); ok {
			shape = pointer.Elem().Underlying()
		}
		receiver = shape.(*types.Struct).Field(index).Type()
	}
	return receiver
}

func receiverField(info *types.Info, expr ast.Expr) machineField {
	switch item := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return machineField{object: info.ObjectOf(item)}
	case *ast.SelectorExpr:
		if selection := info.Selections[item]; selection != nil {
			return receiverField(info, item.X).add(selection.Index())
		}
		return machineField{object: info.Uses[item.Sel]}
	}
	return machineField{}
}

func (field machineField) add(indices []int) machineField {
	for _, index := range indices {
		field.fields += "/" + strconv.Itoa(index)
	}
	return field
}

func bindingBlock(parents []ast.Node) *ast.BlockStmt {
	if statementBlock(parents) != nil {
		return straightLineBlock(parents)
	}
	return publicationBlock(parents)
}

func publicationBlock(parents []ast.Node) *ast.BlockStmt {
	for i := len(parents) - 1; i >= 0; i-- {
		if _, statement := parents[i].(ast.Stmt); statement {
			return straightLineBlock(parents[:i])
		}
	}
	return nil
}

func statementBlock(parents []ast.Node) *ast.BlockStmt {
	if len(parents) == 0 {
		return nil
	}
	block, _ := parents[len(parents)-1].(*ast.BlockStmt)
	return block
}

// A goto can skip an earlier assignment in the same block.
func straightLineBlock(parents []ast.Node) *ast.BlockStmt {
	for i := len(parents) - 1; i >= 0; i-- {
		switch fn := parents[i].(type) {
		case *ast.FuncDecl:
			return blockWithoutGoto(fn.Body, parents)
		case *ast.FuncLit:
			return blockWithoutGoto(fn.Body, parents)
		}
	}
	return nil
}

func blockWithoutGoto(body *ast.BlockStmt, parents []ast.Node) *ast.BlockStmt {
	for node := range ast.Preorder(body) {
		if branch, ok := node.(*ast.BranchStmt); ok && branch.Tok == token.GOTO {
			return nil
		}
	}
	return statementBlock(parents)
}

// A direct write in the return block establishes a known name unless a
// subsequent assignment can replace that machine before the return.
func (facts *machineFacts) fieldNameKnown(field machineField, value machineValue) bool {
	if value.block == nil || field.object == nil {
		return false
	}
	var last nameWrite
	for _, write := range facts.fieldNames[field] {
		dominates := write.block == value.block && write.pos < value.pos
		if dominates && write.pos > last.pos {
			last = write
		}
	}
	if last.name == "" {
		return false
	}
	return !facts.fieldReplaced(field, last.pos, value.pos)
}

func (facts *machineFacts) fieldReplaced(field machineField, lower, upper token.Pos) bool {
	for _, write := range facts.writes[field.object] {
		between := write.pos > lower && write.pos < upper
		if between && write.contains(field) {
			return true
		}
	}
	return false
}

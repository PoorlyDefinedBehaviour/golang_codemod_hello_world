package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"

	"golang.org/x/tools/go/ast/astutil"
)

func main() {
	sourceCode, err := ioutil.ReadFile("./file_to_mod.go")
	if err != nil {
		panic(err)
	}
	fmt.Println("-- original source code --\n\n", string(sourceCode))

	// we want to go from errors.Wrapf(errBadStuff, "some context")
	// to fmt.Errorf("some context %w", errBadStuff)

	updatedSourceCode, err := rewrite(sourceCode)
	if err != nil {
		panic(err)
	}

	fmt.Println("-- updated source code --\n\n", string(updatedSourceCode))
}

// source code -> parse -> transform -> source code
func rewrite(sourceCode []byte) ([]byte, error) {
	// a file set represents a set of source files
	fileSet := token.NewFileSet()

	// parser.ParseComments tells the parser to include comments
	oldAst, err := parser.ParseFile(fileSet, "", sourceCode, parser.ParseComments)
	if err != nil {
		panic(err)
	}

	newAst := astutil.Apply(
		oldAst,
		func(cursor *astutil.Cursor) bool {
			switch value := cursor.Node().(type) {
			case *ast.CallExpr:
				cursor.Replace(handleCallExpr(value))
				return false
			default:
				return true
			}
		},
		nil,
	)

	buffer := bytes.Buffer{}

	err = format.Node(&buffer, fileSet, newAst)
	if err != nil {
		panic(err)
	}

	return buffer.Bytes(), nil
}

func handleCallExpr(expr *ast.CallExpr) *ast.CallExpr {
	name := getCallExprLiteral(expr)

	switch name {
	case "errors.Wrap", "errors.Wrapf":
		return rewriteWrap(expr)

	default:
		return expr
	}
}

func rewriteWrap(expr *ast.CallExpr) *ast.CallExpr {
	// we want to go from errors.Wrapf(errBadStuff, "some context")
	// to fmt.Errorf("some context %w", errBadStuff)
	args := make([]ast.Expr, len(expr.Args)-1)

	// remove error from the first argument position
	// errors.Wrapf("some context")
	copy(args, expr.Args[1:])

	// add error to the last argument position
	// errors.Wrapf("some context", errBadStuff)
	args = append(args, expr.Args[0])

	formatString := args[0].(*ast.BasicLit)

	// given the following string "some context %w"
	// remove the last "
	value := formatString.Value[:len(formatString.Value)-1]

	// add the error format and " to the string
	formatString.Value = value + `: %w"`

	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X: &ast.Ident{
				Name: "fmt",
			},
			Sel: &ast.Ident{
				Name: "Errof",
			},
		},
		Args: args,
	}
}

func getCallExprLiteral(cursor *ast.CallExpr) string {
	selector, ok := cursor.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}

	identifier := selector.X.(*ast.Ident)
	if !ok {
		return ""
	}

	return fmt.Sprintf("%s.%s", identifier.Name, selector.Sel.Name)
}

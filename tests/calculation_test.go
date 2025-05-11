package tests

import (
	"testing"

	"github.com/Andreyka-coder9192/calc_go/internal/application"
	"github.com/Andreyka-coder9192/calc_go/pkg/calculation"
)

// Тесты Compute
func TestCompute(t *testing.T) {
	tests := []struct {
		op   string
		a, b float64
		want float64
	}{
		{"+", 1, 2, 3},
		{"-", 5, 3, 2},
		{"*", 2, 4, 8},
		{"/", 9, 3, 3},
	}
	for _, tt := range tests {
		got, err := calculation.Compute(tt.op, tt.a, tt.b)
		if err != nil {
			t.Errorf("Compute(%q, %v, %v) errored: %v", tt.op, tt.a, tt.b, err)
		}
		if got != tt.want {
			t.Errorf("Compute(%q, %v, %v) = %v; want %v", tt.op, tt.a, tt.b, got, tt.want)
		}
	}
}

// Тесты ParseAST + полный обход AST
func TestParseAndEvalAST(t *testing.T) {
	expr := "(2+3)*4-5/5"
	ast, err := application.ParseAST(expr)
	if err != nil {
		t.Fatalf("ParseAST(%q) failed: %v", expr, err)
	}
	// Встроим простую Eval функцию
	var eval func(n *application.ASTNode) float64
	eval = func(n *application.ASTNode) float64 {
		if n.IsLeaf {
			return n.Value
		}
		l, r := eval(n.Left), eval(n.Right)
		switch n.Operator {
		case "+":
			return l + r
		case "-":
			return l - r
		case "*":
			return l * r
		case "/":
			return l / r
		}
		t.Fatalf("unknown op %q", n.Operator)
		return 0
	}
	if got := eval(ast); got != 5*4-1 {
		t.Errorf("EvalAST(%q) = %v; want %v", expr, got, 5*4-1)
	}
}

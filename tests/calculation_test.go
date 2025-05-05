package tests

import (
	"testing"

	"github.com/Andreyka-coder9192/calc_go/pkg/calculation"
)

func TestCompute(t *testing.T) {
	tests := []struct {
		op        string
		a, b      float64
		expected  float64
		shouldErr bool
	}{
		{"+", 2, 3, 5, false},
		{"-", 5, 3, 2, false},
		{"*", 4, 3, 12, false},
		{"/", 10, 2, 5, false},
		{"/", 10, 0, 0, true},
		{"^", 2, 3, 0, true},
	}
	for _, tc := range tests {
		result, err := calculation.Compute(tc.op, tc.a, tc.b)
		if tc.shouldErr && err == nil {
			t.Errorf("Ожидалась ошибка для операции %s", tc.op)
		}
		if !tc.shouldErr && result != tc.expected {
			t.Errorf("Compute(%s, %f, %f) = %f; ожидалось %f", tc.op, tc.a, tc.b, result, tc.expected)
		}
	}
}

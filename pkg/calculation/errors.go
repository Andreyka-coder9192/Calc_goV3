package calculation

import "errors"

var (
	ErrDivisionByZero  = errors.New("division by zero")
	ErrInvalidOperator = errors.New("invalid operator")
)

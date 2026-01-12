package repayment

import "errors"

// Repayment service specific errors
var (
	// Resource not found errors
	ErrRepaymentNotFound = errors.New("repayment not found")
	ErrLoanNotFound      = errors.New("loan not found")

	// Business logic errors
	ErrInvalidStatusTransition   = errors.New("invalid repayment status transition")
	ErrRepaymentAlreadyPaid      = errors.New("repayment is already paid")
	ErrRepaymentAlreadyWaived    = errors.New("repayment is already waived")
	ErrInsufficientPayment       = errors.New("payment amount is insufficient")
	ErrExcessPayment             = errors.New("payment amount exceeds amount due")
	ErrCannotModifyPaidRepayment = errors.New("cannot modify paid repayment")

	// Validation errors
	ErrInvalidInput         = errors.New("invalid input")
	ErrInvalidAmount        = errors.New("invalid amount")
	ErrInvalidDueDate       = errors.New("invalid due date")
	ErrInvalidStatus        = errors.New("invalid repayment status")
	ErrInvalidPaymentMethod = errors.New("invalid payment method")
)

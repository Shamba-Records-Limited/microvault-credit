package loan

import "errors"

// Loan service specific errors
var (
	// Resource not found errors
	ErrLoanNotFound = errors.New("loan not found")

	// Conflict errors
	ErrLoanNumberAlreadyExists = errors.New("loan number already exists")

	// Business logic errors
	ErrInvalidStatusTransition   = errors.New("invalid loan status transition")
	ErrCannotModifyDeletedLoan   = errors.New("cannot modify deleted loan")
	ErrLoanAlreadyDeleted        = errors.New("loan is already deleted")
	ErrLoanAlreadyDisbursed      = errors.New("loan is already disbursed")
	ErrLoanAlreadyRepaid         = errors.New("loan is already repaid")
	ErrLoanAlreadyDefaulted      = errors.New("loan is already defaulted")
	ErrCannotApprovePendingLoan  = errors.New("can only approve pending loans")
	ErrCannotDisburseUnapproved  = errors.New("can only disburse approved loans")
	ErrCannotRepayNonDisbursed   = errors.New("can only repay disbursed loans")
	ErrUserHasActiveLoan         = errors.New("user already has an active loan")
	ErrExceedsMaxLoanAmount      = errors.New("loan amount exceeds maximum allowed")
	ErrBelowMinLoanAmount        = errors.New("loan amount below minimum required")
	ErrInvalidLoanDuration       = errors.New("invalid loan duration")

	// Validation errors
	ErrInvalidInput           = errors.New("invalid input")
	ErrInvalidAmount          = errors.New("invalid loan amount")
	ErrInvalidInterestRate    = errors.New("invalid interest rate")
	ErrInvalidDuration        = errors.New("invalid duration")
	ErrInvalidStatus          = errors.New("invalid loan status")
	ErrInvalidRepaymentSchedule = errors.New("invalid repayment schedule")
)

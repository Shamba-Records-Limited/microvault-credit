package repository

import (
	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
	"gorm.io/gorm"
)

type Repositories struct {
	// Credit repositories
	Loan               LoanRepository
	LoanProduct        LoanProductRepository
	Repayment          RepaymentRepository
	LoanLimitConfig    LoanLimitConfigRepository
	GlobalLendingLimit GlobalLendingLimitRepository
}

func NewRepositories(db *gorm.DB) (*Repositories, error) {
	if db == nil {
		return nil, pkgErrors.ErrNilDB
	}

	loan, err := NewLoanRepository(db)
	if err != nil {
		return nil, err
	}

	loanProduct, err := NewLoanProductRepository(db)
	if err != nil {
		return nil, err
	}

	repayment, err := NewRepaymentRepository(db)
	if err != nil {
		return nil, err
	}

	loanLimitConfig, err := NewLoanLimitConfigRepository(db)
	if err != nil {
		return nil, err
	}

	globalLendingLimit, err := NewGlobalLendingLimitRepository(db)
	if err != nil {
		return nil, err
	}

	return &Repositories{
		// Credit repositories
		Loan:               loan,
		LoanProduct:        loanProduct,
		Repayment:          repayment,
		LoanLimitConfig:    loanLimitConfig,
		GlobalLendingLimit: globalLendingLimit,
	}, nil
}

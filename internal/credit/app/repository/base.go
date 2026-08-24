package repository

import (
	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
	"gorm.io/gorm"
)

type Repositories struct {
	// Credit repositories
	Loan               LoanRepository
	LoanProduct        LoanProductRepository
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
		LoanLimitConfig:    loanLimitConfig,
		GlobalLendingLimit: globalLendingLimit,
	}, nil
}

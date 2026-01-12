package repository

import (
	pkgErrors "github.com/Shamba-Records-Limited/Microvault/pkg/errors"
	"gorm.io/gorm"
)

type Repositories struct {
	// Credit repositories
	Loan                 LoanRepository
	LoanProduct          LoanProductRepository
	Repayment            RepaymentRepository
	CreditScore          CreditScoreRepository
	CreditFactor         CreditFactorRepository
	Document             DocumentRepository
	CashflowAnalysis     CashflowAnalysisRepository
	TransactionExtracted TransactionExtractedRepository
	FarmRecord           FarmRecordRepository
	CreditScoringFactor  CreditScoringFactorRepository
	RiskTierConfig       RiskTierConfigRepository
	LoanLimitConfig      LoanLimitConfigRepository
	GlobalLendingLimit   GlobalLendingLimitRepository
	CreditConfigAuditLog CreditConfigAuditLogRepository
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

	creditScore, err := NewCreditScoreRepository(db)
	if err != nil {
		return nil, err
	}

	creditFactor, err := NewCreditFactorRepository(db)
	if err != nil {
		return nil, err
	}

	document, err := NewDocumentRepository(db)
	if err != nil {
		return nil, err
	}

	cashflowAnalysis, err := NewCashflowAnalysisRepository(db)
	if err != nil {
		return nil, err
	}

	transactionExtracted, err := NewTransactionExtractedRepository(db)
	if err != nil {
		return nil, err
	}

	farmRecord, err := NewFarmRecordRepository(db)
	if err != nil {
		return nil, err
	}

	creditScoringFactor, err := NewCreditScoringFactorRepository(db)
	if err != nil {
		return nil, err
	}

	riskTierConfig, err := NewRiskTierConfigRepository(db)
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

	creditConfigAuditLog, err := NewCreditConfigAuditLogRepository(db)
	if err != nil {
		return nil, err
	}

	return &Repositories{
		// Credit repositories
		Loan:                 loan,
		LoanProduct:          loanProduct,
		Repayment:            repayment,
		CreditScore:          creditScore,
		CreditFactor:         creditFactor,
		Document:             document,
		CashflowAnalysis:     cashflowAnalysis,
		TransactionExtracted: transactionExtracted,
		FarmRecord:           farmRecord,
		CreditScoringFactor:  creditScoringFactor,
		RiskTierConfig:       riskTierConfig,
		LoanLimitConfig:      loanLimitConfig,
		GlobalLendingLimit:   globalLendingLimit,
		CreditConfigAuditLog: creditConfigAuditLog,
	}, nil
}

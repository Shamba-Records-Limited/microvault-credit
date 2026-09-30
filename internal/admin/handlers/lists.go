package handlers

import (
	"github.com/gofiber/fiber/v2"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/views"
	creditrepo "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	corerepo "github.com/Shamba-Records-Limited/microvault/pkg/repository"
)

// Loans lists loans with a status filter and pagination.
type Loans struct {
	loans creditrepo.LoanRepository
}

func NewLoans(loans creditrepo.LoanRepository) *Loans {
	return &Loans{loans: loans}
}

func (h *Loans) List(c *fiber.Ctx) error {
	page := views.LoansPage{
		Status:   c.Query("status"),
		Page:     queryInt(c, "page", 1),
		PageSize: defaultPageSize,
		Flash:    c.Query("flash"),
	}
	offset := (page.Page - 1) * page.PageSize

	loans, err := h.loans.List(c.UserContext(), page.Status, page.PageSize, offset)
	if err != nil {
		page.Error = "Could not load loans: " + err.Error()
		return render(c, views.Loans(page))
	}
	count, err := h.loans.Count(c.UserContext(), page.Status)
	if err != nil {
		page.Error = "Could not load loans: " + err.Error()
		return render(c, views.Loans(page))
	}

	page.Loans = loans
	page.TotalCount = count
	return render(c, views.Loans(page))
}

// Users lists users with status/KYC filters and pagination.
type Users struct {
	users corerepo.UserRepository
}

func NewUsers(users corerepo.UserRepository) *Users {
	return &Users{users: users}
}

func (h *Users) List(c *fiber.Ctx) error {
	page := views.UsersPage{
		Status:   c.Query("status"),
		KYC:      c.Query("kyc"),
		Page:     queryInt(c, "page", 1),
		PageSize: defaultPageSize,
		Flash:    c.Query("flash"),
	}
	offset := (page.Page - 1) * page.PageSize

	users, err := h.users.ListFiltered(c.UserContext(), page.Status, page.KYC, page.PageSize, offset)
	if err != nil {
		page.Error = "Could not load users: " + err.Error()
		return render(c, views.Users(page))
	}
	count, err := h.users.CountFiltered(c.UserContext(), page.Status, page.KYC)
	if err != nil {
		page.Error = "Could not load users: " + err.Error()
		return render(c, views.Users(page))
	}

	page.Users = users
	page.TotalCount = count
	return render(c, views.Users(page))
}

// Transactions lists transactions with status/type filters and pagination.
type Transactions struct {
	transactions corerepo.TransactionRepository
}

func NewTransactions(transactions corerepo.TransactionRepository) *Transactions {
	return &Transactions{transactions: transactions}
}

func (h *Transactions) List(c *fiber.Ctx) error {
	page := views.TransactionsPage{
		Status:   c.Query("status"),
		Type:     c.Query("type"),
		Page:     queryInt(c, "page", 1),
		PageSize: defaultPageSize,
		Flash:    c.Query("flash"),
	}
	offset := (page.Page - 1) * page.PageSize

	transactions, err := h.transactions.List(c.UserContext(), page.Status, page.Type, page.PageSize, offset)
	if err != nil {
		page.Error = "Could not load transactions: " + err.Error()
		return render(c, views.Transactions(page))
	}
	count, err := h.transactions.Count(c.UserContext(), page.Status, page.Type)
	if err != nil {
		page.Error = "Could not load transactions: " + err.Error()
		return render(c, views.Transactions(page))
	}

	page.Transactions = transactions
	page.TotalCount = count
	return render(c, views.Transactions(page))
}

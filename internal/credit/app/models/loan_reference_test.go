package models

import (
	"strings"
	"testing"

	"github.com/Shamba-Records-Limited/microvault/pkg/loanref"
	"gorm.io/gorm"
)

// The fallback hook must mint the new short format for any creation path that
// did not set a reference explicitly.
func TestBeforeCreate_GeneratesShortReference(t *testing.T) {
	var loan Loan
	if err := loan.BeforeCreate(&gorm.DB{}); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if loan.LoanReference == nil {
		t.Fatal("no reference generated")
	}
	ref := *loan.LoanReference
	if !loanref.Validate(loanref.DefaultPrefix, ref) {
		t.Errorf("reference %q fails loanref validation", ref)
	}
	if len(ref) > 12 {
		t.Errorf("reference %q exceeds Daraja's AccountReference cap", ref)
	}
}

// A caller-supplied reference (the service path, with the configured prefix) is
// never overwritten.
func TestBeforeCreate_PreservesExplicitReference(t *testing.T) {
	explicit := "AA0000000"
	var loan Loan
	loan.LoanReference = &explicit
	if err := loan.BeforeCreate(&gorm.DB{}); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if *loan.LoanReference != explicit {
		t.Errorf("reference overwritten to %q", *loan.LoanReference)
	}
}

// The old timestamp format is gone: no LR-, no hex timestamp.
func TestBeforeCreate_NoLegacyFormat(t *testing.T) {
	for range 50 {
		var loan Loan
		if err := loan.BeforeCreate(&gorm.DB{}); err != nil {
			t.Fatalf("BeforeCreate: %v", err)
		}
		ref := *loan.LoanReference
		if strings.HasPrefix(ref, "LR-") || len(ref) > 9 {
			t.Errorf("reference %q is the legacy format", ref)
		}
	}
}

package loan

import (
	"reflect"
	"testing"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
)

// TestChangedFields_OnlyEmitsSetFields verifies the partial-update mapping does
// not leak nil fields (which would clobber existing columns with NULL).
func TestChangedFields_OnlyEmitsSetFields(t *testing.T) {
	// Nothing set.
	if got := (UpdateLoanRequest{}).changedFields(); len(got) != 0 {
		t.Fatalf("empty request emitted %d fields: %v", len(got), got)
	}

	// Two fields set.
	method := "cash_pickup"
	hash := "abc123"
	req := UpdateLoanRequest{SettlementMethod: &method, RampStellarTxHash: &hash}
	got := req.changedFields()
	if len(got) != 2 {
		t.Fatalf("expected 2 fields, got %d: %v", len(got), got)
	}
	if got["settlement_method"] != &method || got["ramp_stellar_tx_hash"] != &hash {
		t.Fatalf("wrong columns/values: %v", got)
	}
}

// TestChangedFields_EveryPointerFieldIsMapped fills every pointer field on
// UpdateLoanRequest via reflection and asserts changedFields emits one column
// per field, each of which is a real, updatable loan column. This catches the
// recurring failure mode in this repo: a new field added to the request or the
// repository allow-list but not the other, silently dropping writes.
func TestChangedFields_EveryPointerFieldIsMapped(t *testing.T) {
	var req UpdateLoanRequest
	v := reflect.ValueOf(&req).Elem()
	pointerFields := 0
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() != reflect.Ptr || !f.CanSet() {
			continue
		}
		f.Set(reflect.New(f.Type().Elem())) // non-nil zero value
		pointerFields++
	}

	got := req.changedFields()

	if len(got) != pointerFields {
		t.Fatalf("UpdateLoanRequest has %d pointer fields but changedFields emitted %d columns; "+
			"a field is missing from changedFields", pointerFields, len(got))
	}
	for col := range got {
		if !repository.IsUpdatableColumn(col) {
			t.Errorf("changedFields emits column %q which is NOT in the repository allow-list; "+
				"UpdateFields would reject it", col)
		}
	}
}

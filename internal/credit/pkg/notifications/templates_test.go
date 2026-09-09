package notifications

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Shamba-Records-Limited/microvault/pkg/contracts"
	mvnotifications "github.com/Shamba-Records-Limited/microvault/pkg/notifications"
)

const testDialString = "*384*52203#"

type fakeNotifier struct {
	to  string
	msg string
	err error
}

func (f *fakeNotifier) Send(_ context.Context, to, msg string) error {
	f.to, f.msg = to, msg
	return f.err
}

// Every override must encode as GSM-7, or one accented rune halves the segment
// budget for the whole message.
func TestOverridesAreGSM7(t *testing.T) {
	loanNote := mvnotifications.SentinelLoanNotification()
	for lang, tmpl := range LoanOverrides(testDialString) {
		v := reflect.ValueOf(tmpl).Elem()
		for i := range v.NumField() {
			field := v.Field(i)
			if field.IsNil() {
				continue
			}
			msg := field.Interface().(mvnotifications.LoanMessage)(loanNote)
			if _, bad, ok := mvnotifications.GSM7Len(msg); !ok {
				t.Errorf("loan %s %s: %q is outside GSM 03.38:\n%s",
					lang, v.Type().Field(i).Name, bad, msg)
			}
		}
	}

	accountNote := mvnotifications.SentinelAccountNotification()
	for lang, tmpl := range AccountOverrides(testDialString) {
		v := reflect.ValueOf(tmpl).Elem()
		for i := range v.NumField() {
			field := v.Field(i)
			if field.IsNil() {
				continue
			}
			msg := field.Interface().(mvnotifications.AccountMessage)(accountNote)
			if _, bad, ok := mvnotifications.GSM7Len(msg); !ok {
				t.Errorf("account %s %s: %q is outside GSM 03.38:\n%s",
					lang, v.Type().Field(i).Name, bad, msg)
			}
		}
	}
}

// Every language must quote the configured code and nothing else. English and
// Swahili previously disagreed here, so non-English users were told to dial a
// code that does not exist.
func TestEveryLanguageQuotesTheConfiguredDialString(t *testing.T) {
	stale := []string{"*384*1234#", "*789*10#"}

	accountNote := mvnotifications.SentinelAccountNotification()
	for lang, tmpl := range AccountOverrides(testDialString) {
		msg := tmpl.RegistrationSuccess(accountNote)
		if !strings.Contains(msg, testDialString) {
			t.Errorf("%s RegistrationSuccess does not quote the service code:\n%s", lang, msg)
		}
		for _, bad := range stale {
			if strings.Contains(msg, bad) {
				t.Errorf("%s RegistrationSuccess hardcodes %s:\n%s", lang, bad, msg)
			}
		}
	}

	loanNote := mvnotifications.SentinelLoanNotification()
	for lang, tmpl := range LoanOverrides(testDialString) {
		msg := tmpl.RepaymentSoon(loanNote)
		if !strings.Contains(msg, testDialString) {
			t.Errorf("%s RepaymentSoon does not quote the service code:\n%s", lang, msg)
		}
	}
}

// The overrides are partial by design: the loan set leaves the MoneyGram and
// disbursement copy to the platform.
func TestLoanOverridesAreDeliberatelyPartial(t *testing.T) {
	for lang, tmpl := range LoanOverrides(testDialString) {
		if tmpl.Approved != nil {
			t.Errorf("%s: Approved carries no service code and should stay with the platform", lang)
		}
		if tmpl.CashPickupReady != nil {
			t.Errorf("%s: CashPickupReady describes MoneyGram mechanics the platform owns", lang)
		}
		if tmpl.Rejected == nil {
			t.Errorf("%s: Rejected quotes the service code and must be overridden", lang)
		}
	}
}

// Wiring the overrides into the platform notifier must produce branded copy
// for the fields we override and platform copy for the ones we do not.
func TestOverridesWireIntoPlatformNotifier(t *testing.T) {
	fn := &fakeNotifier{}
	n, err := mvnotifications.NewSMSAccountNotifier(fn,
		mvnotifications.WithAccountTemplateSet(AccountOverrides(testDialString)),
	)
	if err != nil {
		t.Fatalf("NewSMSAccountNotifier: %v", err)
	}

	for _, lang := range []string{"en", "sw", "fr"} {
		err := n.NotifyRegistrationSuccess(context.Background(), contracts.AccountNotification{
			Language: lang, FullName: "Alice",
		})
		if err != nil {
			t.Fatalf("%s: %v", lang, err)
		}
		if !strings.Contains(fn.msg, brandName) {
			t.Errorf("%s welcome missing brand:\n%s", lang, fn.msg)
		}
		if !strings.Contains(fn.msg, testDialString) {
			t.Errorf("%s welcome missing service code:\n%s", lang, fn.msg)
		}
	}
}

func TestCreditServiceRendersAndValidates(t *testing.T) {
	fn := &fakeNotifier{}
	svc, err := NewCreditNotificationService(fn, testDialString, nil)
	if err != nil {
		t.Fatalf("NewCreditNotificationService: %v", err)
	}

	err = svc.SendCreditScoreUpdate(context.Background(), CreditNotification{
		PhoneNumber: "254711000111", Score: 720, MaxLoanAmount: 15000, Currency: "KES",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"720", "KES", "15000.00", brandName} {
		if !strings.Contains(fn.msg, want) {
			t.Errorf("score update missing %q:\n%s", want, fn.msg)
		}
	}

	// The currency travels on the notification rather than being hardcoded.
	err = svc.SendCreditScoreUpdate(context.Background(), CreditNotification{
		PhoneNumber: "254711000111", Score: 700, MaxLoanAmount: 120, Currency: "USD",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fn.msg, "KES") {
		t.Errorf("score update hardcodes KES:\n%s", fn.msg)
	}
}

func TestCreditServiceLanguageDispatch(t *testing.T) {
	fn := &fakeNotifier{}
	svc, err := NewCreditNotificationService(fn, testDialString,
		func(context.Context, string) string { return "sw" },
	)
	if err != nil {
		t.Fatalf("NewCreditNotificationService: %v", err)
	}

	// No pinned language -> resolver picks Swahili.
	if err := svc.SendWelcomeMessage(context.Background(), CreditNotification{FullName: "Alice"}); err != nil {
		t.Fatal(err)
	}
	resolved := fn.msg

	if err := svc.SendWelcomeMessage(context.Background(), CreditNotification{
		Language: "en", FullName: "Alice",
	}); err != nil {
		t.Fatal(err)
	}
	if resolved == fn.msg {
		t.Error("resolver-selected (sw) and pinned (en) messages should differ")
	}
}

func TestCreditServiceErrorPropagates(t *testing.T) {
	fn := &fakeNotifier{err: errors.New("transport down")}
	svc, err := NewCreditNotificationService(fn, testDialString, nil)
	if err != nil {
		t.Fatalf("NewCreditNotificationService: %v", err)
	}
	if err := svc.SendSecurityAlert(context.Background(), CreditNotification{Alert: "new device"}); err == nil {
		t.Error("expected transport error to propagate")
	}
}

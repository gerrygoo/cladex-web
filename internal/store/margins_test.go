package store

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"

	cladex "github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/pricing"
)

// marginByName finds a margin option by name, failing the test if it's missing.
func marginByName(t *testing.T, s *Store, ctx context.Context, name string) MarginOption {
	t.Helper()
	opts, err := s.ListMarginOptions(ctx)
	if err != nil {
		t.Fatalf("ListMarginOptions: %v", err)
	}
	for _, o := range opts {
		if o.Name == name {
			return o
		}
	}
	t.Fatalf("no margin option named %q in %+v", name, opts)
	return MarginOption{}
}

func TestMarginOptionsSeeded(t *testing.T) {
	s := newTestStore(t)
	opts, err := s.ListMarginOptions(context.Background())
	if err != nil {
		t.Fatalf("ListMarginOptions: %v", err)
	}
	want := []struct {
		name      string
		value     money.Micros
		isDefault bool
	}{
		{"Estándar", 123_400, true},
		{"Medio", 165_600, false},
		{"Alto", 202_800, false},
		{"CCS & AC", 295_500, false},
	}
	if len(opts) != len(want) {
		t.Fatalf("got %d options, want %d: %+v", len(opts), len(want), opts)
	}
	for i, w := range want {
		o := opts[i]
		if o.Name != w.name || o.ValueMicros != w.value || o.IsDefault != w.isDefault || !o.Active() {
			t.Errorf("option %d = %+v, want %+v", i, o, w)
		}
	}
}

func TestMarginOptionLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	adminID, err := s.CreateUser(ctx, "ana", "Ana", "hash", "admin")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	id, err := s.CreateMarginOption(ctx, "Distribuidor", 80_000, adminID)
	if err != nil {
		t.Fatalf("CreateMarginOption: %v", err)
	}
	if _, err := s.CreateMarginOption(ctx, "Distribuidor", 90_000, adminID); !errors.Is(err, ErrDuplicateMarginName) {
		t.Fatalf("duplicate CreateMarginOption error = %v, want ErrDuplicateMarginName", err)
	}
	if err := s.UpdateMarginOption(ctx, id, "Medio", 90_000, adminID); !errors.Is(err, ErrDuplicateMarginName) {
		t.Fatalf("rename onto an existing name error = %v, want ErrDuplicateMarginName", err)
	}
	if err := s.UpdateMarginOption(ctx, id, "Mayorista", 85_000, adminID); err != nil {
		t.Fatalf("UpdateMarginOption: %v", err)
	}
	if o, err := s.MarginOptionByID(ctx, id); err != nil || o.Name != "Mayorista" || o.ValueMicros != 85_000 {
		t.Fatalf("after update = %+v, %v", o, err)
	}

	// Default moves, never duplicates; the old default can then be retired.
	estandar := marginByName(t, s, ctx, "Estándar")
	if err := s.RetireMarginOption(ctx, estandar.ID, adminID); !errors.Is(err, ErrMarginOptionIsDefault) {
		t.Fatalf("retiring the default error = %v, want ErrMarginOptionIsDefault", err)
	}
	if err := s.SetDefaultMarginOption(ctx, id, adminID); err != nil {
		t.Fatalf("SetDefaultMarginOption: %v", err)
	}
	opts, _ := s.ListMarginOptions(ctx)
	defaults := 0
	for _, o := range opts {
		if o.IsDefault {
			defaults++
			if o.ID != id {
				t.Errorf("default is %q, want Mayorista", o.Name)
			}
		}
	}
	if defaults != 1 {
		t.Fatalf("%d default options, want 1", defaults)
	}
	if err := s.RetireMarginOption(ctx, estandar.ID, adminID); err != nil {
		t.Fatalf("RetireMarginOption: %v", err)
	}
	if o, _ := s.MarginOptionByID(ctx, estandar.ID); o.Active() {
		t.Fatal("retired option still active")
	}
	if err := s.SetDefaultMarginOption(ctx, estandar.ID, adminID); !errors.Is(err, ErrMarginOptionRetired) {
		t.Fatalf("defaulting a retired option error = %v, want ErrMarginOptionRetired", err)
	}
	// Retired options sort after every active one.
	opts, _ = s.ListMarginOptions(ctx)
	if last := opts[len(opts)-1]; last.ID != estandar.ID {
		t.Fatalf("last option = %q, want the retired Estándar", last.Name)
	}
	if err := s.RestoreMarginOption(ctx, estandar.ID, adminID); err != nil {
		t.Fatalf("RestoreMarginOption: %v", err)
	}
	if o, _ := s.MarginOptionByID(ctx, estandar.ID); !o.Active() {
		t.Fatal("restored option still retired")
	}

	if err := s.UpdateMarginOption(ctx, 9999, "X", 1, adminID); !errors.Is(err, ErrMarginOptionNotFound) {
		t.Fatalf("UpdateMarginOption(missing) error = %v, want ErrMarginOptionNotFound", err)
	}
}

// Margin edits move draft prices, so they must be attributed like price edits are.
func TestMarginOptionsAreAudited(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	adminID, err := s.CreateUser(ctx, "ana", "Ana", "hash", "admin")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	actorCtx := WithActor(ctx, Actor{UserID: adminID, Source: SourceWeb})
	medio := marginByName(t, s, ctx, "Medio")
	if err := s.UpdateMarginOption(actorCtx, medio.ID, "Medio", 170_000, adminID); err != nil {
		t.Fatalf("UpdateMarginOption: %v", err)
	}
	entries, err := s.AuditLog(ctx, AuditFilter{TableName: "margin_options", RowKey: strconv.FormatInt(medio.ID, 10)})
	if err != nil || len(entries) == 0 {
		t.Fatalf("AuditLog = %d entries, %v", len(entries), err)
	}
	e := entries[0]
	if e.Op != "update" || e.ActorID == nil || *e.ActorID != adminID ||
		e.OldValues["value_micros"] != float64(165_600) || e.NewValues["value_micros"] != float64(170_000) {
		t.Fatalf("newest margin audit entry = %+v", e)
	}
}

// A draft starts on the default option, keeps a margin change saved with its lines, a
// revision inherits its option, and issuing freezes the option's name and value.
func TestQuoteMarginOption(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	customerID, userID, _, _ := seedQuoteFixtures(t, s, ctx)
	estandar := marginByName(t, s, ctx, "Estándar")
	alto := marginByName(t, s, ctx, "Alto")

	q, err := s.CreateDraftQuote(ctx, customerID, userID, "QA")
	if err != nil {
		t.Fatalf("CreateDraftQuote: %v", err)
	}
	if q.MarginOptionID == nil || *q.MarginOptionID != estandar.ID {
		t.Fatalf("new draft margin = %v, want Estándar (%d)", q.MarginOptionID, estandar.ID)
	}

	if err := s.ReplaceQuoteLines(ctx, q.ID, &alto.ID, nil, nil, pricing.Totals{}); err != nil {
		t.Fatalf("ReplaceQuoteLines: %v", err)
	}
	// nil leaves the saved option alone.
	if err := s.ReplaceQuoteLines(ctx, q.ID, nil, nil, nil, pricing.Totals{}); err != nil {
		t.Fatalf("ReplaceQuoteLines(nil margin): %v", err)
	}
	if q, _ = s.QuoteByID(ctx, q.ID); q.MarginOptionID == nil || *q.MarginOptionID != alto.ID {
		t.Fatalf("saved margin = %v, want Alto (%d)", q.MarginOptionID, alto.ID)
	}

	is := testIssue("sha")
	is.MarginName, is.MarginMicros = alto.Name, alto.ValueMicros
	if err := s.IssueQuote(ctx, q.ID, is); err != nil {
		t.Fatalf("IssueQuote: %v", err)
	}
	q, _ = s.QuoteByID(ctx, q.ID)
	if q.MarginNameSnapshot == nil || *q.MarginNameSnapshot != "Alto" ||
		q.MarginSnapshotMicros == nil || *q.MarginSnapshotMicros != 202_800 {
		t.Fatalf("issued margin snapshot = %v, %v", q.MarginNameSnapshot, q.MarginSnapshotMicros)
	}

	rev, err := s.CreateRevision(ctx, q.ID, userID)
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	if rev.MarginOptionID == nil || *rev.MarginOptionID != alto.ID || rev.MarginNameSnapshot != nil {
		t.Fatalf("revision margin = %v (snapshot %v), want Alto and no snapshot", rev.MarginOptionID, rev.MarginNameSnapshot)
	}
}

// TestMigration0007AssignsDraftMargins runs 0007 over existing quotes: each draft gets
// the option matching its series, issued quotes get none, and default_margin is gone.
func TestMigration0007AssignsDraftMargins(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "test.db")

	old, err := Open(ctx, dsn, migrationsBefore(t, "0007"))
	if err != nil {
		t.Fatalf("open at 0006: %v", err)
	}
	customerID, userID, _, _ := seedQuoteFixtures(t, old, ctx)
	if _, err := old.db.ExecContext(ctx, `
		INSERT INTO quotes (folio, prefix, customer_id, user_id, status)
		VALUES ('QA0001', 'QA', ?1, ?2, 'borrador'), ('QS0001', 'QS', ?1, ?2, 'borrador'),
		       ('QI0001', 'QI', ?1, ?2, 'borrador'), ('QA0002', 'QA', ?1, ?2, 'emitida')`,
		customerID, userID); err != nil {
		t.Fatalf("seed quotes: %v", err)
	}
	if err := old.SetSetting(ctx, "default_margin", "123400", userID); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	old.Close()

	s, err := Open(ctx, dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("open with 0007: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	for folio, want := range map[string]string{"QA0001": "Estándar", "QS0001": "CCS & AC", "QI0001": "Alto"} {
		q, err := s.QuoteByFolio(ctx, folio)
		if err != nil || q == nil || q.MarginOptionID == nil {
			t.Fatalf("QuoteByFolio(%s) = %+v, %v; want a margin", folio, q, err)
		}
		if o, _ := s.MarginOptionByID(ctx, *q.MarginOptionID); o.Name != want {
			t.Errorf("%s margin = %q, want %q", folio, o.Name, want)
		}
	}
	if issued, _ := s.QuoteByFolio(ctx, "QA0002"); issued.MarginOptionID != nil {
		t.Errorf("issued quote got margin %d, want none", *issued.MarginOptionID)
	}
	if v, err := s.SettingValue(ctx, "default_margin"); err != nil || v != "" {
		t.Errorf("default_margin = %q, %v; want deleted", v, err)
	}
}

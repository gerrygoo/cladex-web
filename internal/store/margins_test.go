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

// marginByName finds a general margin option by name, failing the test if it's missing.
func marginByName(t *testing.T, s *Store, ctx context.Context, name string) MarginOption {
	t.Helper()
	return typeMarginByName(t, s, ctx, "", name)
}

// typeMarginByName finds a margin option by name among those of the quote type with
// prefix ("" for the generales).
func typeMarginByName(t *testing.T, s *Store, ctx context.Context, prefix, name string) MarginOption {
	t.Helper()
	var typeID *int64
	if prefix != "" {
		id, err := s.QuoteTypeIDByPrefix(ctx, prefix)
		if err != nil {
			t.Fatalf("QuoteTypeIDByPrefix(%s): %v", prefix, err)
		}
		typeID = &id
	}
	opts, err := s.ListMarginOptions(ctx, typeID)
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
	opts, err := s.ListMarginOptions(context.Background(), nil)
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

	id, err := s.CreateMarginOption(ctx, nil, "Distribuidor", 80_000, adminID)
	if err != nil {
		t.Fatalf("CreateMarginOption: %v", err)
	}
	if _, err := s.CreateMarginOption(ctx, nil, "Distribuidor", 90_000, adminID); !errors.Is(err, ErrDuplicateMarginName) {
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
	opts, _ := s.ListMarginOptions(ctx, nil)
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
	opts, _ = s.ListMarginOptions(ctx, nil)
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
	estandar := typeMarginByName(t, s, ctx, "QA", "Estándar")
	alto := typeMarginByName(t, s, ctx, "QA", "Alto")

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

// Each quote type with catalog products has its own copy of the margins (#82): its
// drafts start on its default, editing one type's margins leaves the others alone, a
// new type uses the generales until it's given margins of its own, and giving them moves
// its drafts onto the copies.
func TestMarginsPerQuoteType(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	customerID, userID, _, _ := seedQuoteFixtures(t, s, ctx)
	qaID, _ := s.QuoteTypeIDByPrefix(ctx, "QA")
	qsID, _ := s.QuoteTypeIDByPrefix(ctx, "QS")
	qlID, _ := s.QuoteTypeIDByPrefix(ctx, "QL")

	own, err := s.MarginTypeIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !own[qaID] || !own[qsID] || own[qlID] {
		t.Fatalf("types with own margins = %v; want QA and QS, not QL (solo líneas libres)", own)
	}
	general := marginByName(t, s, ctx, "Estándar")
	qaEstandar := typeMarginByName(t, s, ctx, "QA", "Estándar")
	qsEstandar := typeMarginByName(t, s, ctx, "QS", "Estándar")
	if qaEstandar.ID == general.ID || qaEstandar.ID == qsEstandar.ID ||
		qaEstandar.ValueMicros != general.ValueMicros || !qaEstandar.IsDefault {
		t.Fatalf("QA Estándar = %+v; want its own default copy of %+v", qaEstandar, general)
	}
	opts, err := s.QuoteMarginOptions(ctx, qaID)
	if err != nil || len(opts) != 4 {
		t.Fatalf("QuoteMarginOptions(QA) = %d options, %v; want 4", len(opts), err)
	}
	for _, o := range opts {
		if o.QuoteTypeID == nil || *o.QuoteTypeID != qaID {
			t.Fatalf("QA builder offers %+v", o)
		}
	}

	q, err := s.CreateDraftQuote(ctx, customerID, userID, "QA")
	if err != nil {
		t.Fatal(err)
	}
	if q.MarginOptionID == nil || *q.MarginOptionID != qaEstandar.ID {
		t.Fatalf("QA draft margin = %v, want QA's Estándar %d", q.MarginOptionID, qaEstandar.ID)
	}

	// Same name in another scope is fine; a default switch stays within its scope.
	dist, err := s.CreateMarginOption(ctx, &qaID, "Distribuidor", 80_000, userID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateMarginOption(ctx, &qsID, "Distribuidor", 90_000, userID); err != nil {
		t.Fatalf("same name on QS: %v", err)
	}
	if _, err := s.CreateMarginOption(ctx, &qaID, "Distribuidor", 90_000, userID); !errors.Is(err, ErrDuplicateMarginName) {
		t.Fatalf("duplicate on QA error = %v, want ErrDuplicateMarginName", err)
	}
	if err := s.SetDefaultMarginOption(ctx, dist, userID); err != nil {
		t.Fatal(err)
	}
	if o, _ := s.MarginOptionByID(ctx, qsEstandar.ID); !o.IsDefault {
		t.Error("QS lost its default when QA's changed")
	}
	if o, _ := s.MarginOptionByID(ctx, general.ID); !o.IsDefault {
		t.Error("the generales lost their default when QA's changed")
	}
	if err := s.UpdateMarginOption(ctx, qaEstandar.ID, "Estándar", 150_000, userID); err != nil {
		t.Fatal(err)
	}
	if o, _ := s.MarginOptionByID(ctx, qsEstandar.ID); o.ValueMicros != general.ValueMicros {
		t.Errorf("QS Estándar = %v after editing QA's", o.ValueMicros)
	}

	// A revision of a QA quote issued on a general option (before 0047) lands on QA's
	// option of the same name.
	alto := marginByName(t, s, ctx, "Alto")
	if err := s.ReplaceQuoteLines(ctx, q.ID, &alto.ID, nil, nil, pricing.Totals{}); err != nil {
		t.Fatal(err)
	}
	if err := s.IssueQuote(ctx, q.ID, testIssue("sha")); err != nil {
		t.Fatal(err)
	}
	rev, err := s.CreateRevision(ctx, q.ID, userID)
	if err != nil {
		t.Fatal(err)
	}
	if qaAlto := typeMarginByName(t, s, ctx, "QA", "Alto"); rev.MarginOptionID == nil || *rev.MarginOptionID != qaAlto.ID {
		t.Fatalf("revision margin = %v, want QA's Alto %d", rev.MarginOptionID, qaAlto.ID)
	}

	// A new type uses the generales until it gets its own.
	newID, err := s.CreateQuoteType(ctx, QuoteType{Prefix: "QEXL", Name: "Exceline"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.CreateDraftQuoteOfType(ctx, customerID, userID, newID)
	if err != nil {
		t.Fatal(err)
	}
	if d.MarginOptionID == nil || *d.MarginOptionID != general.ID {
		t.Fatalf("new type's draft margin = %v, want the general Estándar %d", d.MarginOptionID, general.ID)
	}
	medio := marginByName(t, s, ctx, "Medio")
	if err := s.ReplaceQuoteLines(ctx, d.ID, &medio.ID, nil, nil, pricing.Totals{}); err != nil {
		t.Fatal(err)
	}
	if err := s.UseOwnMargins(ctx, newID, userID); err != nil {
		t.Fatalf("UseOwnMargins: %v", err)
	}
	if err := s.UseOwnMargins(ctx, newID, userID); !errors.Is(err, ErrHasOwnMargins) {
		t.Fatalf("second UseOwnMargins error = %v, want ErrHasOwnMargins", err)
	}
	newMedio := typeMarginByName(t, s, ctx, "QEXL", "Medio")
	if d, _ = s.QuoteByID(ctx, d.ID); d.MarginOptionID == nil || *d.MarginOptionID != newMedio.ID {
		t.Fatalf("draft after UseOwnMargins on %v, want the type's Medio %d", d.MarginOptionID, newMedio.ID)
	}
	if typeMarginByName(t, s, ctx, "QEXL", "Estándar").IsDefault != true {
		t.Error("the copy lost the default")
	}
}

// TestMigration0047 moves drafts onto their type's copy of the option they were on and
// leaves issued quotes on the general one.
func TestMigration0047(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "up.db")
	old, err := Open(ctx, dsn, migrationsBefore(t, "0047"))
	if err != nil {
		t.Fatalf("open at 0046: %v", err)
	}
	issued, userID := projectFixture(t, old)
	var altoID int64
	if err := old.db.QueryRowContext(ctx, `SELECT id FROM margin_options WHERE name = 'Alto'`).Scan(&altoID); err != nil {
		t.Fatal(err)
	}
	draft, err := old.CreateDraftQuote(ctx, issued.CustomerID, userID, "QS")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.db.ExecContext(ctx, `UPDATE quotes SET margin_option_id = ? WHERE id IN (?, ?)`, altoID, draft.ID, issued.ID); err != nil {
		t.Fatal(err)
	}
	old.Close()

	s, err := Open(ctx, dsn, cladex.MigrationsFS)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	qsAlto := typeMarginByName(t, s, ctx, "QS", "Alto")
	if q, _ := s.QuoteByID(ctx, draft.ID); q.MarginOptionID == nil || *q.MarginOptionID != qsAlto.ID {
		t.Errorf("draft margin = %v, want QS's Alto %d", q.MarginOptionID, qsAlto.ID)
	}
	if q, _ := s.QuoteByID(ctx, issued.ID); q.MarginOptionID == nil || *q.MarginOptionID != altoID {
		t.Errorf("issued margin = %v, want the general Alto %d", q.MarginOptionID, altoID)
	}
	for _, prefix := range []string{"QA", "QS", "QI"} {
		if n := len(mustOptions(t, s, ctx, prefix)); n != 4 {
			t.Errorf("%s has %d options, want 4", prefix, n)
		}
	}
}

func mustOptions(t *testing.T, s *Store, ctx context.Context, prefix string) []MarginOption {
	t.Helper()
	id, err := s.QuoteTypeIDByPrefix(ctx, prefix)
	if err != nil {
		t.Fatal(err)
	}
	opts, err := s.ListMarginOptions(ctx, &id)
	if err != nil {
		t.Fatal(err)
	}
	return opts
}

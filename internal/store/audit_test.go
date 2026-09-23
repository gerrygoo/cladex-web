package store

import (
	"context"
	"testing"

	"github.com/gerrygoo/cladex-web/internal/money"
)

func micros(n int64) *money.Micros {
	m := money.Micros(n)
	return &m
}

// seedProduct creates a family and a product without attribution, returning the
// product's id. Callers assert on audit rows written after this point.
func seedProduct(t *testing.T, s *Store, ctx context.Context, sku string) int64 {
	t.Helper()
	familyID, err := s.UpsertFamily(ctx, "CCA", "CCA")
	if err != nil {
		t.Fatalf("UpsertFamily: %v", err)
	}
	id, err := s.CreateProduct(ctx, Product{
		FamilyID: familyID, SKU: sku, Description: "Cable THW 12",
		CostMicros: micros(5_000_000),
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	return id
}

func TestAuditRecordsActorAndDiff(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userID, err := s.CreateUser(ctx, "rodolfo", "Rodolfo Flores", "hashedpw", "admin")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	id := seedProduct(t, s, ctx, "cca-thw-12")

	actorCtx := WithActor(ctx, Actor{UserID: userID, Source: SourceWeb})
	if err := s.UpdateProduct(actorCtx, Product{
		ID: id, FamilyID: 1, SKU: "cca-thw-12", Description: "Cable THW 12",
		CostMicros: micros(5_250_000),
	}); err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}

	entries, err := s.AuditLog(ctx, AuditFilter{TableName: "products", RowKey: "1"})
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d audit entries, want 2 (insert + update)", len(entries))
	}

	upd := entries[0] // newest first
	if upd.Op != "update" {
		t.Fatalf("Op = %q, want update", upd.Op)
	}
	if upd.ActorID == nil || *upd.ActorID != userID {
		t.Fatalf("ActorID = %v, want %d", upd.ActorID, userID)
	}
	if upd.ActorName != "rodolfo" || upd.Source != SourceWeb {
		t.Fatalf("actor = %q/%q, want rodolfo/web", upd.ActorName, upd.Source)
	}
	if got := upd.OldValues["cost_micros"]; got != float64(5_000_000) {
		t.Fatalf("old cost_micros = %v, want 5000000", got)
	}
	if got := upd.NewValues["cost_micros"]; got != float64(5_250_000) {
		t.Fatalf("new cost_micros = %v, want 5250000", got)
	}
	if changed := upd.ChangedFields(); len(changed) != 1 || changed[0] != "cost_micros" {
		t.Fatalf("ChangedFields = %v, want [cost_micros]", changed)
	}

	ins := entries[1]
	if ins.Op != "insert" || ins.OldValues != nil {
		t.Fatalf("insert entry = %+v", ins)
	}
	// seedProduct ran without an actor: the trail must say so rather than borrowing an
	// identity from a neighbouring write.
	if ins.ActorID != nil || ins.Source != "" {
		t.Fatalf("unattributed insert recorded actor %v/%q", ins.ActorID, ins.Source)
	}
}

// A write with no actor must not inherit the previous write's identity.
func TestAuditActorDoesNotLeakBetweenWrites(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userID, err := s.CreateUser(ctx, "rodolfo", "Rodolfo Flores", "hashedpw", "admin")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	id := seedProduct(t, s, ctx, "cca-thw-12")

	attributed := WithActor(ctx, Actor{UserID: userID, Source: SourceWeb})
	if err := s.SoftDeleteProduct(attributed, id); err != nil {
		t.Fatalf("SoftDeleteProduct: %v", err)
	}
	// Now an unattributed write, as a background job or the sqlite3 shell would make.
	if err := s.UpdateProduct(ctx, Product{
		ID: id, FamilyID: 1, SKU: "cca-thw-12", Description: "Cable THW 12 (unattributed)",
		CostMicros: micros(5_500_000),
	}); err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}

	entries, err := s.AuditLog(ctx, AuditFilter{TableName: "products", RowKey: "1"})
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d audit entries, want 3", len(entries))
	}
	if entries[0].ActorID != nil {
		t.Fatalf("unattributed write inherited actor %v", *entries[0].ActorID)
	}
	if entries[1].ActorID == nil || *entries[1].ActorID != userID {
		t.Fatalf("soft delete lost its actor: %+v", entries[1])
	}
}

// The users trigger must record that a password changed without ever writing the hash.
func TestAuditNeverStoresPasswordHash(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userID, err := s.CreateUser(ctx, "rodolfo", "Rodolfo Flores", "originalhash", "vendedor")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := s.UpdatePassword(WithActor(ctx, Actor{UserID: userID, Source: SourceWeb}), userID, "newhash"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	if err := s.SetUserRole(ctx, userID, "admin"); err != nil {
		t.Fatalf("SetUserRole: %v", err)
	}

	entries, err := s.AuditLog(ctx, AuditFilter{TableName: "users"})
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}
	for _, e := range entries {
		for _, vals := range []map[string]any{e.OldValues, e.NewValues} {
			for k, v := range vals {
				if k == "password_hash" {
					t.Fatalf("audit entry %d stored a password hash", e.ID)
				}
				if s, ok := v.(string); ok && (s == "originalhash" || s == "newhash") {
					t.Fatalf("audit entry %d leaked a password hash via %q", e.ID, k)
				}
			}
		}
	}

	roleChange, pwChange := entries[0], entries[1]
	if got := roleChange.ChangedFields(); len(got) != 1 || got[0] != "role" {
		t.Fatalf("role change ChangedFields = %v, want [role]", got)
	}
	if got := pwChange.NewValues["password_changed"]; got != float64(1) {
		t.Fatalf("password_changed = %v, want 1", got)
	}
	if got := roleChange.NewValues["password_changed"]; got != float64(0) {
		t.Fatalf("role-only change reported password_changed = %v, want 0", got)
	}
	// password_changed=0 is a marker, not a diff — it must not show up as a changed field.
	if got := pwChange.ChangedFields(); len(got) != 1 || got[0] != "password_changed" {
		t.Fatalf("password change ChangedFields = %v, want [password_changed]", got)
	}
}

// settings is keyed by TEXT, so its audit rows key on the setting name.
func TestAuditSettingsUseTextRowKey(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userID, err := s.CreateUser(ctx, "rodolfo", "Rodolfo Flores", "hashedpw", "admin")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	actorCtx := WithActor(ctx, Actor{UserID: userID, Source: SourceWeb})
	if err := s.SetSetting(actorCtx, "fx_rate_micros", "17500000", userID); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if err := s.SetSetting(actorCtx, "fx_rate_micros", "18000000", userID); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	entries, err := s.AuditLog(ctx, AuditFilter{TableName: "settings", RowKey: "fx_rate_micros"})
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].OldValues["value"] != "17500000" || entries[0].NewValues["value"] != "18000000" {
		t.Fatalf("FX change diff = %v -> %v", entries[0].OldValues["value"], entries[0].NewValues["value"])
	}
}

func TestAuditLogIsAppendOnly(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProduct(t, s, ctx, "cca-thw-12")

	if _, err := s.db.ExecContext(ctx, `UPDATE audit_log SET actor_id = 99 WHERE id = 1`); err == nil {
		t.Fatal("UPDATE on audit_log succeeded; want it rejected")
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM audit_log WHERE id = 1`); err == nil {
		t.Fatal("DELETE on audit_log succeeded; want it rejected")
	}
}

// A failed write must leave no audit row: the stamp and the write share one transaction.
func TestAuditRollsBackWithFailedWrite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProduct(t, s, ctx, "cca-thw-12")

	before, err := s.AuditLog(ctx, AuditFilter{TableName: "products"})
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}
	// Duplicate SKU violates the UNIQUE index.
	if _, err := s.CreateProduct(ctx, Product{
		FamilyID: 1, SKU: "cca-thw-12", Description: "duplicate",
	}); err == nil {
		t.Fatal("CreateProduct with duplicate SKU succeeded; want UNIQUE violation")
	}

	after, err := s.AuditLog(ctx, AuditFilter{TableName: "products"})
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("failed write left %d audit rows behind", len(after)-len(before))
	}
}

// The units work landed on main alongside this trail; conversions carry the same
// money weight as prices, since a wrong "1 rollo = 100 m" silently multiplies a line.
func TestAuditCoversUnitConversions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userID, err := s.CreateUser(ctx, "rodolfo", "Rodolfo Flores", "hashedpw", "admin")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	productID := seedProduct(t, s, ctx, "cca-thw-12")

	units, err := s.ListUnits(ctx)
	if err != nil {
		t.Fatalf("ListUnits: %v", err)
	}
	byCode := map[string]int64{}
	for _, u := range units {
		byCode[u.Code] = u.ID
	}

	actorCtx := WithActor(ctx, Actor{UserID: userID, Source: SourceWeb})
	convID, err := s.CreateConversion(actorCtx, productID, byCode["rollo"], byCode["m"], 100_000_000)
	if err != nil {
		t.Fatalf("CreateConversion: %v", err)
	}
	if err := s.DeleteConversion(actorCtx, convID); err != nil {
		t.Fatalf("DeleteConversion: %v", err)
	}

	entries, err := s.AuditLog(ctx, AuditFilter{TableName: "product_unit_conversions"})
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (insert + delete)", len(entries))
	}
	if entries[0].Op != "delete" || entries[0].NewValues != nil {
		t.Fatalf("delete entry = %+v", entries[0])
	}
	if entries[0].OldValues["rate_micros"] != float64(100_000_000) {
		t.Fatalf("deleted rate = %v, want 100000000", entries[0].OldValues["rate_micros"])
	}
	if entries[1].ActorID == nil || *entries[1].ActorID != userID {
		t.Fatalf("insert actor = %v, want %d", entries[1].ActorID, userID)
	}
}

// products gained unit_id in the units migration; a column the trail does not capture
// is a silent hole, so assert it is there.
func TestAuditCoversProductUnitColumn(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProduct(t, s, ctx, "cca-thw-12")

	entries, err := s.AuditLog(ctx, AuditFilter{TableName: "products"})
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no audit entries for products")
	}
	if _, ok := entries[0].NewValues["unit_id"]; !ok {
		t.Fatalf("products audit entry has no unit_id key: %v", entries[0].NewValues)
	}
}

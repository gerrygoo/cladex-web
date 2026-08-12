package store

import (
	"context"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	adminID, err := s.CreateUser(ctx, "ana", "Ana", "hashedpw", "admin")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if v, err := s.SettingValue(ctx, "fx_rate"); err != nil || v != "" {
		t.Fatalf("SettingValue(unset) = %q, %v; want \"\", nil", v, err)
	}

	if err := s.SetSetting(ctx, "fx_rate", "18500000", adminID); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if v, err := s.SettingValue(ctx, "fx_rate"); err != nil || v != "18500000" {
		t.Fatalf("SettingValue = %q, %v; want \"18500000\", nil", v, err)
	}

	// Overwrite records the new value and the admin who set it.
	if err := s.SetSetting(ctx, "fx_rate", "19000000", adminID); err != nil {
		t.Fatalf("SetSetting overwrite: %v", err)
	}
	var updatedBy int64
	if err := s.db.QueryRowContext(ctx, `SELECT updated_by FROM settings WHERE key = 'fx_rate'`).Scan(&updatedBy); err != nil {
		t.Fatalf("check updated_by: %v", err)
	}
	if updatedBy != adminID {
		t.Fatalf("updated_by = %d, want %d", updatedBy, adminID)
	}

	values, err := s.SettingValues(ctx, []string{"fx_rate", "copper_price", "default_margin"})
	if err != nil {
		t.Fatalf("SettingValues: %v", err)
	}
	if values["fx_rate"] != "19000000" {
		t.Fatalf("SettingValues[fx_rate] = %q, want 19000000", values["fx_rate"])
	}
	if _, ok := values["copper_price"]; ok {
		t.Fatalf("SettingValues should omit unset keys, got %+v", values)
	}
}

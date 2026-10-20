package facturoporti

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web/internal/appenv"
)

// testdata holds one of the SAT's public test CSDs (Facturoporti's Csd-Prueba.zip):
// ESCUELA KEMPER URGATE, RFC EKU9003173C9, password 12345678a. iia040805dz4.cer is
// another test company's certificate, for a key that doesn't match.
const testPassword = "12345678a"

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseCSD(t *testing.T) {
	cer, key := readTestdata(t, "eku9003173c9.cer"), readTestdata(t, "eku9003173c9.key")
	csd, err := ParseCSD(cer, key, testPassword)
	if err != nil {
		t.Fatalf("ParseCSD: %v", err)
	}
	if csd.RFC != "EKU9003173C9" || csd.Number != "30001000000500003416" || csd.NotAfter.Year() != 2027 {
		t.Errorf("CSD = RFC %q, number %q, expires %v", csd.RFC, csd.Number, csd.NotAfter)
	}
	if err := csd.CheckValidAt(time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Errorf("CheckValidAt(2026): %v", err)
	}
	if err := csd.CheckValidAt(time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "venció") {
		t.Errorf("CheckValidAt(2028) = %v, want expired", err)
	}

	for name, c := range map[string]struct {
		cer, key []byte
		password string
		want     string
	}{
		"wrong password":   {cer, key, "12345678b", "contraseña"},
		"other cert":       {readTestdata(t, "iia040805dz4.cer"), key, testPassword, "no corresponde"},
		"key is not a key": {cer, cer, testPassword, "no se puede leer"},
		"cert is a key":    {key, key, testPassword, "certificado"},
	} {
		if _, err := ParseCSD(c.cer, c.key, c.password); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one mentioning %q", name, err, c.want)
		}
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	files := map[string]string{
		"FACTUROPORTI_TOKEN_FILE": filepath.Join(dir, "token"),
		"CSD_CER_FILE":            filepath.Join(dir, "cer"),
		"CSD_KEY_FILE":            filepath.Join(dir, "key"),
		"CSD_PASSWORD_FILE":       filepath.Join(dir, "password"),
	}
	getenv := func(k string) string { return files[k] }
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	staging, _ := appenv.Parse("staging")
	prodDefault, _ := appenv.Parse("")
	prod, _ := appenv.Parse("production")

	if _, st := Load(staging, getenv, now); st.Enabled || st.Problem != "no está configurada" || !st.Sandbox {
		t.Errorf("no files: %+v", st)
	}
	write("token", []byte("tok\n"))
	if _, st := Load(staging, getenv, now); st.Enabled || !strings.Contains(st.Problem, "falta: certificado (.cer), llave privada (.key), contraseña") {
		t.Errorf("token only: %+v", st)
	}
	os.Remove(filepath.Join(dir, "token"))
	write("cer", readTestdata(t, "eku9003173c9.cer"))
	write("key", readTestdata(t, "eku9003173c9.key"))
	write("password", []byte(testPassword+"\n"))
	// Without the token, the CSD is still checked and shown.
	if _, st := Load(staging, getenv, now); st.Enabled || st.Problem != "falta: token de Facturoporti" || st.RFC != "EKU9003173C9" {
		t.Errorf("CSD without token: %+v", st)
	}
	write("token", []byte("tok\n"))

	cfg, st := Load(staging, getenv, now)
	if !st.Enabled || cfg.Token != "tok" || cfg.BaseURL != SandboxURL || st.RFC != "EKU9003173C9" || cfg.CSD.Password != testPassword {
		t.Fatalf("staging: %+v %+v", cfg, st)
	}
	// Production only when the instance says so.
	if cfg, _ := Load(prodDefault, getenv, now); cfg.BaseURL != SandboxURL {
		t.Errorf("unset APP_ENV talks to %s", cfg.BaseURL)
	}
	if cfg, st := Load(prod, getenv, now); cfg.BaseURL != ProductionURL || st.Sandbox {
		t.Errorf("APP_ENV=production talks to %s", cfg.BaseURL)
	}
	// An expired CSD turns it off but still says whose it is.
	if _, st := Load(staging, getenv, time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)); st.Enabled || st.RFC != "EKU9003173C9" || !strings.Contains(st.Problem, "venció") {
		t.Errorf("expired: %+v", st)
	}
	write("password", []byte("wrong"))
	if _, st := Load(staging, getenv, now); st.Enabled || !strings.Contains(st.Problem, "contraseña") {
		t.Errorf("wrong password: %+v", st)
	}
}

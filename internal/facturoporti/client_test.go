package facturoporti

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func testCSD(t *testing.T) *CSD {
	t.Helper()
	csd, err := ParseCSD(readTestdata(t, "eku9003173c9.cer"), readTestdata(t, "eku9003173c9.key"), testPassword)
	if err != nil {
		t.Fatal(err)
	}
	return csd
}

// testRequest is a one-concepto factura from the SAT's test emisor to a test receptor.
func testRequest(folio string) StampRequest {
	return StampRequest{
		DatosGenerales: DatosGenerales{Version: "4.0", CFDI: "Factura", TipoCFDI: "Ingreso", OpcionDecimales: 2, NumeroDecimales: 2},
		Encabezado: Encabezado{
			Emisor:   Emisor{RFC: "EKU9003173C9", NombreRazonSocial: "ESCUELA KEMPER URGATE", RegimenFiscal: "601"},
			Receptor: Receptor{RFC: "IIA040805DZ4", NombreRazonSocial: "INDISTRIA ILUMINADORA DE ALMACENES", UsoCFDI: "G01", RegimenFiscal: "601", Direccion: Direccion{CodigoPostal: "62661"}},
			Fecha:    time.Now().In(time.FixedZone("CST", -6*3600)).Format("2006-01-02T15:04:05"),
			Serie:    "TEST", Folio: folio, MetodoPago: "PUE", FormaPago: "03", Moneda: "MXN", LugarExpedicion: "42501",
			// 1,250 m at $39.129876: importe 48,912.35, IVA 7,825.98.
			SubTotal: Dec(4891235, 2), Total: Dec(5673833, 2),
		},
		Conceptos: []Concepto{{
			Cantidad: Dec(1250000, 3), CodigoUnidad: "MTR", Unidad: "Metro", CodigoProducto: "26121600",
			Producto: "Cable CCS 30% 7#8 LC DSA (1/0 AWG)", PrecioUnitario: Dec(39129876, 6), Importe: Dec(4891235, 2),
			ObjetoDeImpuesto: "02",
			Impuestos:        []Impuesto{{TipoImpuesto: 1, Impuesto: 2, Factor: 1, Base: Dec(4891235, 2), Tasa: IVA16, ImpuestoImporte: Dec(782598, 2)}},
		}},
	}
}

func TestDecAndMarshal(t *testing.T) {
	for _, c := range []struct {
		v      int64
		places int
		want   string
	}{{11600, 2, "116.00"}, {5, 2, "0.05"}, {39129876, 6, "39.129876"}, {1250000, 3, "1250.000"}, {-150, 2, "-1.50"}, {7, 0, "7"}} {
		if got := Dec(c.v, c.places); string(got) != c.want {
			t.Errorf("Dec(%d, %d) = %s, want %s", c.v, c.places, got, c.want)
		}
	}
	a, _ := Marshal(testRequest("1"))
	b, _ := Marshal(testRequest("1"))
	if string(a) != string(b) {
		t.Error("Marshal is not deterministic")
	}
	for _, want := range []string{`"Tasa":0.160000`, `"PrecioUnitario":39.129876`, `"Total":56738.33`, `"Producto":"Cable CCS 30% 7#8 LC DSA (1/0 AWG)"`} {
		if !strings.Contains(string(a), want) {
			t.Errorf("request missing %s: %s", want, a)
		}
	}
	if strings.Contains(string(a), "CSD") {
		t.Error("a stored request carries CSD fields")
	}
}

func fakeClient(t *testing.T, f *Fake) *Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, Sandbox: true, Token: "tok", CSD: testCSD(t), HTTP: srv.Client()}
}

func TestClientAgainstFake(t *testing.T) {
	f := NewFake()
	c := fakeClient(t, f)
	ctx := context.Background()
	stored, _ := Marshal(testRequest("1"))

	r := c.Stamp(ctx, stored)
	if r.Outcome != Done || r.UUID == "" || !strings.Contains(r.XML, `Folio="1"`) || strings.Contains(r.Raw, "cfdi:Comprobante") {
		t.Fatalf("Stamp = %+v", r)
	}
	// Lost answer: Unknown, and resending the same bytes gets the same stamp.
	stored2, _ := Marshal(testRequest("2"))
	f.Drop = true
	if r2 := c.Stamp(ctx, stored2); r2.Outcome != Unknown {
		t.Fatalf("dropped Stamp = %+v", r2)
	}
	r2 := c.Stamp(ctx, stored2)
	if r2.Outcome != Done || f.Stamps() != 2 {
		t.Fatalf("resend = %+v, stamps %d", r2, f.Stamps())
	}
	if again := c.Stamp(ctx, stored2); again.UUID != r2.UUID || f.Stamps() != 2 {
		t.Errorf("identical resend stamped again: %s vs %s", again.UUID, r2.UUID)
	}
	// A refusal carries the SAT's text.
	f.Refuse = "CFDI40147 - El RFC del receptor no existe"
	if r3 := c.Stamp(ctx, stored); r3.Outcome != Refused || !strings.Contains(r3.Message, "CFDI40147") {
		t.Errorf("refused Stamp = %+v", r3)
	}
	// Bad token: refused, nothing stamped.
	bad := *c
	bad.Token = ""
	bad.HTTP = &http.Client{Transport: headerless{c.HTTP.Transport}}
	if r4 := bad.Stamp(ctx, stored); r4.Outcome != Refused {
		t.Errorf("unauthorized Stamp = %+v", r4)
	}

	cr := c.Cancel(ctx, CancelRequest{RFCEmisor: "EKU9003173C9", RFCReceptor: "IIA040805DZ4", UUID: r.UUID, Total: Dec(5673833, 2), Motivo: "02"})
	if cr.Outcome != Done || cr.Acuse == "" {
		t.Errorf("Cancel = %+v", cr)
	}
	if st := c.Status(ctx, r.UUID, "EKU9003173C9", "IIA040805DZ4", Dec(5673833, 2)); st.Outcome != Done || st.Estado != "Cancelado" {
		t.Errorf("Status = %+v", st)
	}
	f.CancelPending = true
	if cr := c.Cancel(ctx, CancelRequest{UUID: r2.UUID, Motivo: "02"}); cr.Outcome != Pending {
		t.Errorf("pending Cancel = %+v", cr)
	}
	if cr := c.Cancel(ctx, CancelRequest{UUID: r2.UUID, Motivo: "01"}); cr.Outcome != Refused {
		t.Errorf("motivo 01 without sustitución = %+v", cr)
	}
}

// headerless drops the Authorization header, as a missing token would.
type headerless struct{ rt http.RoundTripper }

func (h headerless) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Del("Authorization")
	return h.rt.RoundTrip(r)
}

func TestClientTimeoutIsUnknown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(300 * time.Millisecond) }))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Token: "t", CSD: testCSD(t), HTTP: &http.Client{Timeout: 50 * time.Millisecond}}
	stored, _ := Marshal(testRequest("1"))
	if r := c.Stamp(context.Background(), stored); r.Outcome != Unknown {
		t.Errorf("timeout = %+v", r)
	}
}

// TestSandbox stamps and cancels against Facturoporti's real sandbox with its public test
// user, when FACTUROPORTI_SANDBOX=1 (it needs the network, so it is off by default).
func TestSandbox(t *testing.T) {
	if os.Getenv("FACTUROPORTI_SANDBOX") != "1" {
		t.Skip("set FACTUROPORTI_SANDBOX=1 to run against testapi.facturoporti.com.mx")
	}
	resp, err := http.Get(SandboxURL + "/token/crear?Usuario=PruebasTimbrado&Password=" + url.QueryEscape("@Notiene1"))
	if err != nil {
		t.Fatal(err)
	}
	var tok struct{ Codigo, Token string }
	json.NewDecoder(resp.Body).Decode(&tok)
	resp.Body.Close()
	if tok.Token == "" {
		t.Fatalf("token: %+v", tok)
	}
	c := &Client{BaseURL: SandboxURL, Sandbox: true, Token: tok.Token, CSD: testCSD(t), HTTP: &http.Client{Timeout: 90 * time.Second}}
	ctx := context.Background()
	stored, _ := Marshal(testRequest(time.Now().Format("150405")))
	r := c.Stamp(ctx, stored)
	if r.Outcome != Done {
		t.Fatalf("Stamp = %+v", r)
	}
	t.Logf("stamped %s", r.UUID)
	if again := c.Stamp(ctx, stored); again.UUID != r.UUID {
		t.Errorf("identical resend: %s, want %s", again.UUID, r.UUID)
	}
	cr := c.Cancel(ctx, CancelRequest{RFCEmisor: "EKU9003173C9", RFCReceptor: "IIA040805DZ4", UUID: r.UUID, Total: Dec(5673833, 2), Motivo: "02"})
	if cr.Outcome != Done && cr.Outcome != Pending {
		t.Errorf("Cancel = %+v", cr)
	}
}

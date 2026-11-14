package avisos

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gerrygoo/cladex-web/internal/store"
)

func TestSupplierSlots(t *testing.T) {
	loc := MexicoCity()
	for in, want := range map[string]string{
		"2026-11-12T10:00": "2026-11-12T11:00 2026-11-12T13:00 2026-11-12T18:00", // Thursday morning
		"2026-11-12T14:59": "2026-11-12T15:59 2026-11-12T17:59 2026-11-12T18:00",
		"2026-11-12T15:00": "2026-11-12T16:00 2026-11-12T17:00", // from 15:00: two
		"2026-11-12T17:00": "2026-11-12T18:00 2026-11-12T19:00", // Emilio's example
		"2026-11-12T18:30": "2026-11-13T09:00 2026-11-13T11:00 2026-11-13T18:00", // evening → next morning
		"2026-11-12T06:00": "2026-11-12T09:00 2026-11-12T11:00 2026-11-12T18:00", // before 8:00
		"2026-11-13T19:00": "2026-11-16T09:00 2026-11-16T11:00 2026-11-16T18:00", // Friday night → Monday
		"2026-11-14T11:00": "2026-11-16T09:00 2026-11-16T11:00 2026-11-16T18:00", // Saturday
	} {
		at, _ := time.ParseInLocation(slotLayout, in, loc)
		var got []string
		for _, s := range supplierSlots(at, loc) {
			got = append(got, s.Format(slotLayout))
		}
		if strings.Join(got, " ") != want {
			t.Errorf("supplierSlots(%s) = %v, want %s", in, got, want)
		}
	}
}

// ocFixture is the seguimiento fixture with QA0001's client O.C. received on Thursday
// 2026-11-12 at 10:00, Mexico City. Gerardo (admin) has C and F on; Ana (vendedor) asked
// for both, and only C took.
func ocFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	ctx := context.Background()
	if err := f.s.SetUserAvisos(ctx, f.gerry, store.RoleAdmin, []string{store.AvisoOCRecibida, store.AvisoOCProveedor}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.SetUserAvisos(ctx, f.ana, store.RoleVendedor, []string{store.AvisoOCRecibida, store.AvisoOCProveedor}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.ReceiveOC(ctx, f.project, f.ana, store.OC{Number: "77", Date: "2026-11-12"},
		&store.NewFile{Filename: "oc.pdf", ContentType: "application/pdf", Data: []byte("%PDF")}, ""); err != nil {
		t.Fatal(err)
	}
	f.received(t, "2026-11-12T16:00:00.000Z")
	return f
}

// received records, as the audit trail would, that the client O.C. was received at at.
func (f *fixture) received(t *testing.T, at string) {
	t.Helper()
	f.exec(t, `INSERT INTO audit_log (table_name, row_key, op, old_values, new_values, at)
		VALUES ('projects', ?, 'update', '{"status":"prospecto"}', '{"status":"en_curso"}', ?)`, f.project, at)
}

// handle runs one of the service's handlers as if at local (Mexico City) and returns its
// result and the notifications it queued, by user.
func (f *fixture) handle(t *testing.T, h func(context.Context, store.Job) (string, error), job store.Job, local string) (string, map[int64][]string) {
	t.Helper()
	at, err := time.ParseInLocation(slotLayout, local, f.svc.loc)
	if err != nil {
		t.Fatal(err)
	}
	f.svc.now = func() time.Time { return at }
	var before int
	f.raw.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM notifications`).Scan(&before)
	result, err := h(context.Background(), job)
	if err != nil {
		t.Fatalf("%s at %s: %v", job.Payload, local, err)
	}
	rows, err := f.raw.Query(`SELECT user_id, subject FROM notifications WHERE id > ?`, before)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[int64][]string{}
	for rows.Next() {
		var id int64
		var subject string
		rows.Scan(&id, &subject)
		got[id] = append(got[id], subject)
	}
	return result, got
}

func TestOCRecibida(t *testing.T) {
	f := ocFixture(t)
	var payload string
	if err := f.raw.QueryRow(`SELECT payload FROM jobs WHERE kind = ?`, OCJobKind).Scan(&payload); err != nil {
		t.Fatalf("receiving the O.C. queued no aviso job: %v", err)
	}
	job := store.Job{Kind: OCJobKind, Payload: payload}

	_, got := f.handle(t, f.svc.HandleOC, job, "2026-11-12T10:00")
	if len(got) != 2 || got[f.gerry][0] != "Orden colocada 100%: QA0001 · EMSA" || len(got[f.ana]) != 1 {
		t.Errorf("C queued %v, want one each for Gerardo and Ana", got)
	}
	var body string
	f.raw.QueryRow(`SELECT body_text FROM notifications WHERE user_id = ?`, f.gerry).Scan(&body)
	for _, want := range []string{"Hola, Gerardo.", "orden colocada al 100%", "O.C. del cliente 77 del 12/11/2026",
		"Siguiente paso: generar y mandar la orden de compra al proveedor.", "https://cotizador.example/proyectos/QA0001"} {
		if !strings.Contains(body, want) {
			t.Errorf("C lacks %q:\n%s", want, body)
		}
	}
	if len(f.jobs.keys) != 3 {
		t.Errorf("F reminders queued: %v, want 3", f.jobs.keys)
	}
	// A retried job doesn't send twice.
	if result, got := f.handle(t, f.svc.HandleOC, job, "2026-11-12T10:01"); len(got) != 0 || !strings.Contains(result, "ya se había enviado") {
		t.Errorf("retry: %q %v", result, got)
	}
}

func TestOCProveedor(t *testing.T) {
	f := ocFixture(t)
	step := func(n int, runAt string) store.Job {
		return store.Job{Kind: SupplierJobKind, RunAt: runAt,
			Payload: `{"project_id":` + itoa(f.project) + `,"received_at":"2026-11-12T16:00:00Z","step":` + itoa(int64(n)) + `}`}
	}

	// Without a supplier O.C. at all, the 11:00 reminder goes, only to the admin.
	_, got := f.handle(t, f.svc.HandleSupplier, step(1, "2026-11-12T17:00:00.000Z"), "2026-11-12T11:00")
	if len(got) != 1 || got[f.gerry][0] != "O.C. al proveedor sin enviar: QA0001 · EMSA" {
		t.Errorf("step 1 queued %v, want one for Gerardo", got)
	}
	if result, got := f.handle(t, f.svc.HandleSupplier, step(1, "2026-11-12T17:00:00.000Z"), "2026-11-12T11:05"); len(got) != 0 || !strings.Contains(result, "ya se había") {
		t.Errorf("step 1 again: %q %v", result, got)
	}
	// A draft isn't sent yet.
	f.exec(t, `INSERT INTO purchase_orders (project_id, quote_id, created_by) SELECT id, (SELECT MAX(id) FROM quotes), ? FROM projects`, f.ana)
	if _, got := f.handle(t, f.svc.HandleSupplier, step(2, "2026-11-12T19:00:00.000Z"), "2026-11-12T13:00"); len(got) != 1 {
		t.Errorf("with a draft, step 2 queued %v", got)
	}
	// Confirmed: the rest stop.
	f.exec(t, `UPDATE purchase_orders SET status = 'confirmada', number = 'OC-1', seq = 1, document = '{}'`)
	if result, got := f.handle(t, f.svc.HandleSupplier, step(3, "2026-11-13T00:00:00.000Z"), "2026-11-12T18:00"); len(got) != 0 || !strings.Contains(result, "ya está confirmada") {
		t.Errorf("once confirmed: %q %v", result, got)
	}
	// Nor for an O.C. received again after being undone: that one has its own.
	f.exec(t, `DELETE FROM purchase_orders`)
	f.received(t, "2026-11-12T20:00:00.000Z")
	if result, got := f.handle(t, f.svc.HandleSupplier, step(3, "2026-11-13T00:00:00.000Z"), "2026-11-12T18:00"); len(got) != 0 || !strings.Contains(result, "con esa orden") {
		t.Errorf("after a new receipt: %q %v", result, got)
	}
}

func TestResumenPerdidos(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.s.SetUserAvisos(ctx, f.ana, store.RoleVendedor, []string{store.AvisoResumenPerdidos})
	if err := f.s.LoseProject(ctx, f.project, f.ana, store.StageProspecto, "Compró con otro proveedor."); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE projects SET lost_at = '2026-10-20T18:00:00.000Z', name = 'Capuchinas'`)
	job := store.Job{Kind: PerdidosJobKind, Payload: `{"month":"2026-11"}`}

	// November 1st 2026 is a Sunday: the summary is on Monday the 2nd at 9:00.
	if got := perdidosSlot(time.Date(2026, 11, 1, 0, 0, 0, 0, f.svc.loc), f.svc.loc).Format(slotLayout); got != "2026-11-02T09:00" {
		t.Errorf("perdidosSlot = %s", got)
	}
	_, got := f.handle(t, f.svc.HandlePerdidos, job, "2026-11-02T09:00")
	if len(got) != 1 || got[f.ana][0] != "Proyectos perdidos en octubre de 2026" {
		t.Fatalf("queued %v", got)
	}
	var body string
	f.raw.QueryRow(`SELECT body_text FROM notifications WHERE user_id = ?`, f.ana).Scan(&body)
	for _, want := range []string{"En octubre de 2026 se perdieron 1 proyecto", "QA0001 · Capuchinas · EMSA · vendedor Ana",
		"Motivo: Compró con otro proveedor.", "https://cotizador.example/proyectos/QA0001"} {
		if !strings.Contains(body, want) {
			t.Errorf("summary lacks %q:\n%s", want, body)
		}
	}
	if f.jobs.keys[len(f.jobs.keys)-1] != "2026-12" {
		t.Errorf("next summary queued: %v", f.jobs.keys)
	}
	if result, got := f.handle(t, f.svc.HandlePerdidos, job, "2026-11-02T09:05"); len(got) != 0 || !strings.Contains(result, "ya se había") {
		t.Errorf("again: %q %v", result, got)
	}

	// A month with none says so; one that runs too late sends nothing.
	_, got = f.handle(t, f.svc.HandlePerdidos, store.Job{Payload: `{"month":"2026-12"}`}, "2026-12-01T09:00")
	var none string
	f.raw.QueryRow(`SELECT body_text FROM notifications ORDER BY id DESC LIMIT 1`).Scan(&none)
	if len(got) != 1 || !strings.Contains(none, "En noviembre de 2026 no se perdió ningún proyecto.") {
		t.Errorf("empty month: %v\n%s", got, none)
	}
	if result, got := f.handle(t, f.svc.HandlePerdidos, store.Job{Payload: `{"month":"2027-01"}`}, "2027-01-08T09:00"); len(got) != 0 || !strings.Contains(result, "tarde") {
		t.Errorf("late: %q %v", result, got)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

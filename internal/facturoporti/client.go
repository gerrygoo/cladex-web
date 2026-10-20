package facturoporti

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client calls Facturoporti with one instance's token and CSD (see Load).
type Client struct {
	BaseURL string
	Sandbox bool
	Token   string
	CSD     *CSD
	HTTP    *http.Client
}

// NewClient is the client for a loaded Config. Stamping can take several seconds (the
// sandbox took 4.5 s for a first stamp), so the timeout is generous; past it the outcome
// is Unknown, never assumed failed.
func NewClient(cfg *Config) *Client {
	return &Client{BaseURL: cfg.BaseURL, Sandbox: cfg.Sandbox, Token: cfg.Token, CSD: cfg.CSD,
		HTTP: &http.Client{Timeout: 90 * time.Second}}
}

// Outcome is what a call did, as far as we can tell.
type Outcome int

const (
	// Unknown: no answer, or one we couldn't read. Whatever was asked may or may not have
	// happened.
	Unknown Outcome = iota
	// Done: it happened (stamped, or cancelled).
	Done
	// Refused: Facturoporti or the SAT answered no. Nothing happened.
	Refused
	// Pending: a cancellation waits on the receptor's acceptance.
	Pending
)

func (o Outcome) String() string {
	return [...]string{"unknown", "done", "refused", "pending"}[o]
}

// StampResult is a stamp attempt's outcome. Raw is the response with the stamped XML and
// the PDF taken out (kept elsewhere or not at all), for the event log.
type StampResult struct {
	Outcome   Outcome
	UUID      string
	XML       string
	EmailSent bool
	Code      string
	Message   string // what to show a person: the PAC's or the SAT's error, or why it is unknown
	Raw       string
}

type stampResponse struct {
	Estatus struct {
		Codigo             string
		Descripcion        string
		InformacionTecnica string
		Fecha              string
	}
	CfdiTimbrado *struct {
		Respuesta *struct {
			UUID          string
			Cfdixml       string
			EmailEnviado  string
			Pdf           string
			TimbreXML     string
			NoCertificado string
			Fecha         string
		}
	}
}

// Stamp sends a stored request (Marshal's bytes, without the CSD) with this instance's
// CSD filled in. Facturoporti answers a byte-identical request with the stamp it already
// made (checked in the sandbox, 2026-10-20: same UUID and XML, while a request that
// differs even in its Fecha gets a new stamp). So the stored bytes are the idempotency
// key: after an Unknown outcome, calling Stamp again with the same stored bytes can't
// stamp twice, and a re-validated (re-dated) request can.
func (c *Client) Stamp(ctx context.Context, stored []byte) StampResult {
	var req StampRequest
	if err := json.Unmarshal(stored, &req); err != nil {
		return StampResult{Outcome: Refused, Message: "la solicitud guardada no se puede leer: " + err.Error()}
	}
	req.DatosGenerales.CSD = base64.StdEncoding.EncodeToString(c.CSD.Cert)
	req.DatosGenerales.LlavePrivada = base64.StdEncoding.EncodeToString(c.CSD.Key)
	req.DatosGenerales.CSDPassword = c.CSD.Password
	body, err := Marshal(req)
	if err != nil {
		return StampResult{Outcome: Refused, Message: err.Error()}
	}
	status, resp, err := c.post(ctx, "/servicios/timbrar/json", body)
	if r, done := classifyHTTP(status, resp, err); done {
		return StampResult{Outcome: r.outcome, Message: r.message, Raw: r.raw}
	}
	var sr stampResponse
	if err := json.Unmarshal(resp, &sr); err != nil {
		return StampResult{Outcome: Unknown, Message: "respuesta ilegible de Facturoporti", Raw: truncate(string(resp))}
	}
	res := StampResult{Code: sr.Estatus.Codigo, Message: errorText(sr.Estatus.Descripcion, sr.Estatus.InformacionTecnica)}
	var r = struct{ uuid, xml, email string }{}
	if sr.CfdiTimbrado != nil && sr.CfdiTimbrado.Respuesta != nil {
		p := sr.CfdiTimbrado.Respuesta
		r.uuid, r.xml, r.email = p.UUID, p.Cfdixml, p.EmailEnviado
		p.Cfdixml, p.Pdf, p.TimbreXML = "", "", ""
	}
	if raw, err := json.Marshal(sr); err == nil {
		res.Raw = string(raw)
	}
	switch {
	case sr.Estatus.Codigo == "000" && r.uuid != "" && r.xml != "":
		res.Outcome, res.UUID, res.XML = Done, strings.ToUpper(r.uuid), r.xml
		res.EmailSent = strings.EqualFold(r.email, "true")
		res.Message = ""
	case sr.Estatus.Codigo == "000":
		res.Outcome, res.Message = Unknown, "Facturoporti dijo que timbró pero no mandó el timbre"
	case sr.Estatus.Codigo == "":
		res.Outcome, res.Message = Unknown, "respuesta de Facturoporti sin código"
	default:
		res.Outcome = Refused
	}
	return res
}

// CancelRequest asks to cancel a stamped CFDI. Motivo is the SAT's c_Motivo (01 with
// Sustitucion, the UUID that replaces it; 02, 03, 04 without).
type CancelRequest struct {
	RFCEmisor   string
	RFCReceptor string
	UUID        string
	Total       json.Number
	Motivo      string
	Sustitucion string
}

// CancelResult is a cancellation's outcome: Done when the SAT shows it cancelled, Pending
// when it waits on the receptor. Acuse is the SAT's acknowledgement XML.
type CancelResult struct {
	Outcome Outcome
	Code    string
	Message string
	Estado  string
	Acuse   string
	Raw     string
}

// Cancel asks for a cancellation with this instance's CSD. Asking again for one already
// requested is harmless: the SAT answers with its current state.
func (c *Client) Cancel(ctx context.Context, r CancelRequest) CancelResult {
	body, err := json.Marshal(map[string]any{
		"rfcEmisor": r.RFCEmisor, "rfcReceptor": r.RFCReceptor, "uuid": r.UUID, "total": r.Total,
		"motivo": r.Motivo, "folioFiscalSustitucion": r.Sustitucion,
		"certificado":  base64.StdEncoding.EncodeToString(c.CSD.Cert),
		"llavePrivada": base64.StdEncoding.EncodeToString(c.CSD.Key),
		"password":     c.CSD.Password,
	})
	if err != nil {
		return CancelResult{Outcome: Refused, Message: err.Error()}
	}
	status, resp, err := c.post(ctx, "/servicios/cancelar/csd", body)
	if hr, done := classifyHTTP(status, resp, err); done {
		return CancelResult{Outcome: hr.outcome, Message: hr.message, Raw: hr.raw}
	}
	var cr struct {
		Codigo, Mensaje, Acuse, CodigoEstatus, EsCancelable, Estado, EstatusCancelacion string
	}
	if err := json.Unmarshal(resp, &cr); err != nil {
		return CancelResult{Outcome: Unknown, Message: "respuesta ilegible de Facturoporti", Raw: truncate(string(resp))}
	}
	res := CancelResult{Code: cr.Codigo, Message: cr.Mensaje, Estado: cr.Estado, Acuse: cr.Acuse}
	acuse := cr.Acuse
	cr.Acuse = ""
	if raw, err := json.Marshal(cr); err == nil {
		res.Raw = string(raw)
	}
	res.Acuse = acuse
	switch {
	case cr.Codigo != "000":
		res.Outcome = Refused
	case strings.EqualFold(cr.Estado, "Cancelado"):
		res.Outcome, res.Message = Done, ""
	default:
		res.Outcome, res.Message = Pending, cancelState(cr.Estado, cr.EstatusCancelacion)
	}
	return res
}

// StatusResult is the SAT's view of a stamped CFDI.
type StatusResult struct {
	Outcome            Outcome // Done when the SAT answered
	Estado             string  // Vigente, Cancelado, No Encontrado
	EstatusCancelacion string  // e.g. En proceso, Solicitud rechazada
	EsCancelable       string
	Message            string
	Raw                string
}

// Status asks the SAT, through Facturoporti, for a CFDI's state. It only works for real
// CFDIs: the sandbox's aren't known to the SAT (checked 2026-10-20, code 001).
func (c *Client) Status(ctx context.Context, uuid, rfcEmisor, rfcReceptor string, total json.Number) StatusResult {
	body, _ := json.Marshal([]map[string]any{{"uuid": uuid, "rfcEmisor": rfcEmisor, "rfcReceptor": rfcReceptor, "total": string(total)}})
	status, resp, err := c.post(ctx, "/validar/estatuscfdi", body)
	if hr, done := classifyHTTP(status, resp, err); done {
		return StatusResult{Outcome: hr.outcome, Message: hr.message, Raw: hr.raw}
	}
	var sr struct {
		Codigo, Mensaje string
		Cfdi            []struct{ UUID, CodigoEstatus, EsCancelable, Estado, EstatusCancelacion string }
	}
	if err := json.Unmarshal(resp, &sr); err != nil {
		return StatusResult{Outcome: Unknown, Message: "respuesta ilegible de Facturoporti", Raw: truncate(string(resp))}
	}
	res := StatusResult{Raw: truncate(string(resp)), Message: sr.Mensaje}
	if sr.Codigo != "000" || len(sr.Cfdi) == 0 {
		res.Outcome = Refused
		return res
	}
	res.Outcome = Done
	res.Estado, res.EstatusCancelacion, res.EsCancelable = sr.Cfdi[0].Estado, sr.Cfdi[0].EstatusCancelacion, sr.Cfdi[0].EsCancelable
	return res
}

func (c *Client) post(ctx context.Context, path string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	return resp.StatusCode, b, err
}

type httpOutcome struct {
	outcome Outcome
	message string
	raw     string
}

// classifyHTTP settles what the transport alone tells: no answer or a server error is
// Unknown; a refused request (bad token, invalid JSON) is Refused, since Facturoporti
// didn't process it. done is false for a 200, whose body decides.
func classifyHTTP(status int, body []byte, err error) (httpOutcome, bool) {
	switch {
	case err != nil:
		msg := "sin respuesta de Facturoporti: " + err.Error()
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "Timeout") {
			msg = "Facturoporti no respondió a tiempo"
		}
		return httpOutcome{Unknown, msg, ""}, true
	case status == http.StatusOK:
		return httpOutcome{}, false
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return httpOutcome{Refused, "Facturoporti no aceptó el token (revisa la configuración)", truncate(string(body))}, true
	case status >= 400 && status < 500:
		return httpOutcome{Refused, fmt.Sprintf("Facturoporti rechazó la solicitud (HTTP %d): %s", status, truncate(string(body))), truncate(string(body))}, true
	default:
		return httpOutcome{Unknown, fmt.Sprintf("error de Facturoporti (HTTP %d)", status), truncate(string(body))}, true
	}
}

func errorText(desc, tech string) string {
	if tech == "" || strings.EqualFold(tech, "ok") {
		return desc
	}
	return desc + ": " + tech
}

func cancelState(estado, estatus string) string {
	if estatus != "" {
		return estado + " · " + estatus
	}
	return estado
}

func truncate(s string) string {
	if len(s) > 4000 {
		return s[:4000] + "…"
	}
	return s
}

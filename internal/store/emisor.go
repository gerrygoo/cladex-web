package store

import (
	"context"
	"regexp"
	"strings"
)

// Emisor is Cladex as the issuer of its CFDIs, kept in settings and edited by an admin in
// Configuración del sistema. It must match the CSD's RFC; that is checked when a
// comprobante is validated, since the CSD is not in the database.
type Emisor struct {
	RFC          string
	Name         string // razón social as registered with the SAT, without the régimen de capital
	TaxRegime    string // c_RegimenFiscal
	PostalCode   string // lugar de expedición
	SerieFactura string
	SeriePago    string
}

var emisorKeys = []string{"emisor_rfc", "emisor_nombre", "emisor_regimen", "emisor_cp", "factura_serie", "pago_serie"}

// Default series for facturas and comprobantes de pago until an admin picks others.
const (
	DefaultSerieFactura = "F"
	DefaultSeriePago    = "P"
)

// EmisorSettings returns the emisor as stored; unset fields are empty, and the series
// default to F and P.
func (s *Store) EmisorSettings(ctx context.Context) (Emisor, error) {
	v, err := s.SettingValues(ctx, emisorKeys)
	if err != nil {
		return Emisor{}, err
	}
	e := Emisor{RFC: v["emisor_rfc"], Name: v["emisor_nombre"], TaxRegime: v["emisor_regimen"], PostalCode: v["emisor_cp"],
		SerieFactura: v["factura_serie"], SeriePago: v["pago_serie"]}
	if e.SerieFactura == "" {
		e.SerieFactura = DefaultSerieFactura
	}
	if e.SeriePago == "" {
		e.SeriePago = DefaultSeriePago
	}
	return e, nil
}

var (
	rfcPattern   = regexp.MustCompile(`^[A-ZÑ&]{3,4}[0-9]{6}[A-Z0-9]{3}$`)
	cpPattern    = regexp.MustCompile(`^[0-9]{5}$`)
	seriePattern = regexp.MustCompile(`^[A-Z]{1,10}$`)
)

// ValidRFC reports whether rfc has the shape of an RFC: 12 characters for a persona
// moral, 13 for a persona física. Whether it exists is the SAT's to say.
func ValidRFC(rfc string) bool { return rfcPattern.MatchString(rfc) }

// ValidPostalCode reports whether cp is five digits.
func ValidPostalCode(cp string) bool { return cpPattern.MatchString(cp) }

// Errors returns the emisor's problems by field (rfc, nombre, regimen, cp, serie_factura,
// serie_pago), empty when it can issue CFDIs.
func (e Emisor) Errors() map[string]string {
	errs := map[string]string{}
	if !ValidRFC(e.RFC) {
		errs["rfc"] = "Escribe el RFC de 12 o 13 caracteres, en mayúsculas."
	}
	if strings.TrimSpace(e.Name) == "" {
		errs["nombre"] = "Escribe la razón social como aparece en la constancia de situación fiscal."
	}
	if !ValidSATCode(TaxRegimes, e.TaxRegime) {
		errs["regimen"] = "Elige el régimen fiscal."
	}
	if !ValidPostalCode(e.PostalCode) {
		errs["cp"] = "Escribe el código postal de 5 dígitos."
	}
	if !seriePattern.MatchString(e.SerieFactura) {
		errs["serie_factura"] = "La serie son de 1 a 10 letras, p. ej. F."
	}
	if !seriePattern.MatchString(e.SeriePago) {
		errs["serie_pago"] = "La serie son de 1 a 10 letras, p. ej. P."
	}
	if len(errs) == 0 && e.SerieFactura == e.SeriePago {
		errs["serie_pago"] = "Usa una serie distinta a la de facturas."
	}
	return errs
}

// SetEmisor saves the emisor. The caller validates it first (Emisor.Errors).
func (s *Store) SetEmisor(ctx context.Context, e Emisor, userID int64) error {
	values := []string{e.RFC, strings.TrimSpace(e.Name), e.TaxRegime, e.PostalCode, e.SerieFactura, e.SeriePago}
	for i, k := range emisorKeys {
		if err := s.SetSetting(ctx, k, values[i], userID); err != nil {
			return err
		}
	}
	return nil
}

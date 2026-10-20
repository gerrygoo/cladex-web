package store

// SATCode is one entry of a SAT catalog: its code and its official description.
type SATCode struct {
	Code, Description string
}

// TaxRegimes is the SAT's c_RegimenFiscal catalog for CFDI 4.0, the receptor's régimen
// fiscal (issue #17). migrations/0022_customer_fiscal_fields.sql lists the same codes.
var TaxRegimes = []SATCode{
	{"601", "General de Ley Personas Morales"},
	{"603", "Personas Morales con Fines no Lucrativos"},
	{"605", "Sueldos y Salarios e Ingresos Asimilados a Salarios"},
	{"606", "Arrendamiento"},
	{"607", "Régimen de Enajenación o Adquisición de Bienes"},
	{"608", "Demás ingresos"},
	{"610", "Residentes en el Extranjero sin Establecimiento Permanente en México"},
	{"611", "Ingresos por Dividendos (socios y accionistas)"},
	{"612", "Personas Físicas con Actividades Empresariales y Profesionales"},
	{"614", "Ingresos por intereses"},
	{"615", "Régimen de los ingresos por obtención de premios"},
	{"616", "Sin obligaciones fiscales"},
	{"620", "Sociedades Cooperativas de Producción que optan por diferir sus ingresos"},
	{"621", "Incorporación Fiscal"},
	{"622", "Actividades Agrícolas, Ganaderas, Silvícolas y Pesqueras"},
	{"623", "Opcional para Grupos de Sociedades"},
	{"624", "Coordinados"},
	{"625", "Régimen de las Actividades Empresariales con ingresos a través de Plataformas Tecnológicas"},
	{"626", "Régimen Simplificado de Confianza"},
}

// CFDIUses is the SAT's c_UsoCFDI catalog for CFDI 4.0, a customer's default uso de CFDI
// (issue #17). Which uses a given régimen may take is the PAC's to check when stamping.
var CFDIUses = []SATCode{
	{"G01", "Adquisición de mercancías"},
	{"G02", "Devoluciones, descuentos o bonificaciones"},
	{"G03", "Gastos en general"},
	{"I01", "Construcciones"},
	{"I02", "Mobiliario y equipo de oficina por inversiones"},
	{"I03", "Equipo de transporte"},
	{"I04", "Equipo de cómputo y accesorios"},
	{"I05", "Dados, troqueles, moldes, matrices y herramental"},
	{"I06", "Comunicaciones telefónicas"},
	{"I07", "Comunicaciones satelitales"},
	{"I08", "Otra maquinaria y equipo"},
	{"D01", "Honorarios médicos, dentales y gastos hospitalarios"},
	{"D02", "Gastos médicos por incapacidad o discapacidad"},
	{"D03", "Gastos funerales"},
	{"D04", "Donativos"},
	{"D05", "Intereses reales efectivamente pagados por créditos hipotecarios (casa habitación)"},
	{"D06", "Aportaciones voluntarias al SAR"},
	{"D07", "Primas por seguros de gastos médicos"},
	{"D08", "Gastos de transportación escolar obligatoria"},
	{"D09", "Depósitos en cuentas para el ahorro, primas que tengan como base planes de pensiones"},
	{"D10", "Pagos por servicios educativos (colegiaturas)"},
	{"S01", "Sin efectos fiscales"},
	{"CP01", "Pagos"},
	{"CN01", "Nómina"},
}

// ValidSATCode reports whether code is in catalog; "" (not set) is valid.
func ValidSATCode(catalog []SATCode, code string) bool {
	if code == "" {
		return true
	}
	for _, c := range catalog {
		if c.Code == code {
			return true
		}
	}
	return false
}

// SATLabel is a code with its description, "601 - General de Ley Personas Morales", or
// the code alone if it isn't in catalog.
func SATLabel(catalog []SATCode, code string) string {
	for _, c := range catalog {
		if c.Code == code {
			return c.Code + " - " + c.Description
		}
	}
	return code
}

// PaymentForms is the SAT's c_FormaPago catalog: how a factura was (P.U.E.) or a payment
// is paid. 99 "Por definir" is what a P.P.D. factura carries, never a payment.
var PaymentForms = []SATCode{
	{"01", "Efectivo"},
	{"02", "Cheque nominativo"},
	{"03", "Transferencia electrónica de fondos"},
	{"04", "Tarjeta de crédito"},
	{"05", "Monedero electrónico"},
	{"06", "Dinero electrónico"},
	{"08", "Vales de despensa"},
	{"12", "Dación en pago"},
	{"13", "Pago por subrogación"},
	{"14", "Pago por consignación"},
	{"15", "Condonación"},
	{"17", "Compensación"},
	{"23", "Novación"},
	{"24", "Confusión"},
	{"25", "Remisión de deuda"},
	{"26", "Prescripción o caducidad"},
	{"27", "A satisfacción del acreedor"},
	{"28", "Tarjeta de débito"},
	{"29", "Tarjeta de servicios"},
	{"30", "Aplicación de anticipos"},
	{"31", "Intermediario pagos"},
	{"99", "Por definir"},
}

// CancelMotives is the SAT's c_Motivo for cancelling a CFDI. 01 needs the UUID of the
// CFDI that replaces it.
var CancelMotives = []SATCode{
	{"01", "Comprobante emitido con errores con relación"},
	{"02", "Comprobante emitido con errores sin relación"},
	{"03", "No se llevó a cabo la operación"},
	{"04", "Operación nominativa relacionada en una factura global"},
}

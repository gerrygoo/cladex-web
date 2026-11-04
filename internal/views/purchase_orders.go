package views

import (
	"strconv"

	"github.com/gerrygoo/cladex-web/internal/store"
)

// Órdenes de compra a proveedor (issue #65): the proyecto's section and the O.C.'s own
// page, a borrador's form or a confirmed O.C.'s summary.

// POPath is an O.C.'s page; action, when not "", one of its actions under it.
func POPath(folio string, id int64, action string) string {
	p := "/proyectos/" + folio + "/ordenes-de-compra/" + strconv.FormatInt(id, 10)
	if action != "" {
		p += "/" + action
	}
	return p
}

// POPDFPath is the O.C.'s PDF, shown in the browser; download asks to save it.
func POPDFPath(folio string, id int64, download bool) string {
	p := POPath(folio, id, "pdf")
	if download {
		p += "?descargar=1"
	}
	return p
}

// ProjectPOs is the proyecto page's "Orden de compra al proveedor" section. Show is
// false before the client's O.C. is in (and nothing was generated). CanGenerate offers
// "Generar orden de compra"; Hint says why it isn't offered, when it would help.
type ProjectPOs struct {
	Show        bool
	Orders      []store.PurchaseOrder
	CanGenerate bool
	Another     bool // QL with O.C.s already: the button generates another one
	Hint        string
}

// POLineView is one partida on an O.C.'s page. On a borrador it is one of the quote's
// lines, Checked when it is in the O.C.; NoCost marks one without a cost to buy at, and
// TakenBy names the O.C. of the proyecto that already has it (QL).
type POLineView struct {
	QuoteLineNo int
	Description string
	Qty         string
	KgPerUnit   string
	Kg          string
	PricePerKg  string
	UnitPrice   string
	Total       string
	Checked     bool
	NoCost      bool
	TakenBy     string
}

// POFormValues are a borrador's fields as the form shows them.
type POFormValues struct {
	SupplierID         string
	DeliveryAddress    string
	DeliveryReferences string
	Currency           string
	PaymentForm        string
	CreditDays         string
	PaymentOther       string
}

// POBillingView is the Datos de facturación block as an O.C. prints it.
type POBillingView struct {
	Name, RFC, Regime, Address, CFDIUse string
	// Missing is set when something is blank: the O.C. can't be confirmed until an admin
	// completes it in Configuración del sistema.
	Missing bool
}

// PurchaseOrderPage is an O.C.'s page.
type PurchaseOrderPage struct {
	Project store.Project
	PO      store.PurchaseOrder
	// Editable: a borrador and the viewer is the proyecto's vendedor or an admin.
	Editable  bool
	IsAdmin   bool
	FreeLines bool
	ShowKg    bool
	Lines     []POLineView
	Values    POFormValues
	Suppliers []store.POSupplierOption
	Billing   POBillingView
	// Conditions are what the PDF prints, for a confirmed O.C.'s summary.
	Conditions           []string
	Subtotal, IVA, Total string
	Errors               map[string]string
	ErrorMsg, SuccessMsg string
	// The new-supplier form of a QL borrador (admins): its values, its errors, the
	// suppliers it may duplicate, and whether it shows open.
	NewSupplier       SupplierFormValues
	NewSupplierErrors map[string]string
	Similar           []store.SupplierRow
	NewSupplierOpen   bool
	// Download is the PDF to save right after confirming, "" otherwise.
	Download string
}

// POStatusLabel is how a status reads.
func POStatusLabel(status string) string {
	if l, ok := store.POStatusLabels[status]; ok {
		return l
	}
	return status
}

// poNewSupplierError is the new-supplier form's error for field, whose name in the
// form carries the "nuevo_" prefix.
func poField(field string) string { return "nuevo_" + field }

func (p PurchaseOrderPage) title() string {
	if p.PO.Number != "" {
		return "Orden de compra " + p.PO.Number
	}
	return "Orden de compra al proveedor: " + p.PO.Label()
}

func (p PurchaseOrderPage) supplierOffered(o store.POSupplierOption) bool {
	return !o.Inactive || p.Values.SupplierID == strconv.FormatInt(o.ID, 10)
}

func creditDaysAttr(o store.POSupplierOption) string {
	if o.CreditDays == nil {
		return ""
	}
	return strconv.FormatInt(*o.CreditDays, 10)
}

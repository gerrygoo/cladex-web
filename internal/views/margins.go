package views

import (
	"strconv"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/pricing"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// MarginPercent renders a margin fraction in micros as a plain percentage, without the
// sign: 123_400 -> "12.34". It's the same number admins type in /ajustes.
func MarginPercent(m money.Micros) string {
	return money.Micros(int64(m) * 100).String()
}

// MarginLabel is how a margin option reads everywhere it's shown: "Estándar (12.34%)".
func MarginLabel(name string, value money.Micros) string {
	return name + " (" + MarginPercent(value) + "%)"
}

// MarginChoice is one <option> of the quote builder's margin dropdown.
type MarginChoice struct {
	ID       int64
	Label    string
	Pct      string // the option's percentage, for prefilling the custom fields
	Selected bool
}

// MarginPicker is the quote builder's margin dropdown: every active option, plus the
// quote's own option when it has since been retired (so the dropdown still shows what
// the draft is on). Error explains why the draft can't be priced or issued with its
// current selection; the handler renders it inside the line fragment so a recalculation
// clears it.
type MarginPicker struct {
	Choices    []MarginChoice
	NoneChosen bool
	Error      string

	// AllowCustom is set on CCS quotes, the only ones that can use a margin of their
	// own; only then are the "Personalizado" option and the two margin fields shown.
	AllowCustom bool
	// Custom is set when the draft is on a margin of its own ("Personalizado") rather
	// than a menu option; CustomPct holds the percentage typed (or derived) for it.
	// When the catalog has the copper material (HasCopper), the same margin is also
	// offered as that material's sale price per kg: CopperName and CopperCost say what
	// the price is measured against, and CopperPrice is the price CustomPct implies.
	Custom      bool
	CustomPct   string
	HasCopper   bool
	CopperName  string
	CopperCost  string
	CopperPrice string
}

// NewMarginPicker builds the dropdown for a draft whose selected option is selectedID
// (nil: none).
func NewMarginPicker(opts []store.MarginOption, selectedID *int64) MarginPicker {
	var p MarginPicker
	found := false
	for _, o := range opts {
		selected := selectedID != nil && o.ID == *selectedID
		if !o.Active() && !selected {
			continue
		}
		label := MarginLabel(o.Name, o.ValueMicros)
		if !o.Active() {
			label += " — retirado"
		}
		p.Choices = append(p.Choices, MarginChoice{ID: o.ID, Label: label, Pct: MarginPercent(o.ValueMicros), Selected: selected})
		found = found || selected
	}
	p.NoneChosen = !found
	return p
}

// SetMarginDisplay fills the percentage field with margin and, when the catalog has the
// copper material, the price per kg that margin implies.
func (p *MarginPicker) SetMarginDisplay(margin money.Micros, copper *store.Material) {
	p.CustomPct = MarginPercent(margin)
	if copper == nil {
		return
	}
	if price, err := pricing.PriceFromMargin(copper.PriceMicros, margin); err == nil {
		p.CopperPrice = CopperPriceText(price)
	}
}

// CCSFamilyName is the product family whose quotes can carry a custom margin.
const CCSFamilyName = "CCS & AC"

// CustomMarginValue is the margin dropdown's value for "Personalizado".
const CustomMarginValue = "custom"

// CustomMarginName is the name a custom margin carries in snapshots and pricing inputs.
const CustomMarginName = "Personalizado"

// CopperPriceText renders a sale price per kg for the copper field: whole centavos, no
// currency formatting, so it round-trips through an <input>.
func CopperPriceText(m money.Micros) string {
	return money.Micros(money.RoundHalfUp(int64(m), 10_000) * 10_000).String()
}

func marginChoiceValue(id int64) string {
	return strconv.FormatInt(id, 10)
}

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

// ShortMarginPercent is MarginPercent rounded half up to two decimals: a margin derived
// from a price per kg (issue #38) is kept in full, 124_668 -> "12.47".
func ShortMarginPercent(m money.Micros) string {
	return MarginPercent(money.Micros(money.RoundHalfUp(int64(m), 100) * 100))
}

// MarginLabel is how a margin option reads everywhere it's shown: "Estándar (12.34%)".
func MarginLabel(name string, value money.Micros) string {
	return name + " (" + ShortMarginPercent(value) + "%)"
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

	// AllowCustom is set on catalog series (QA, QS, QI), which can use a margin of their
	// own; only then is the "Otro margen" option offered. Its percentage field shows
	// while it is picked, and on CCS quotes always, read-only for a menu option, next to
	// the copper price.
	AllowCustom bool
	// Custom is set when the draft is on a margin of its own ("Otro margen") rather
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

// SetMarginDisplay fills the percentage field with margin, to two decimals, and, when
// the catalog has the copper material, the price per kg: price when the margin was typed
// as one, else the price margin implies.
func (p *MarginPicker) SetMarginDisplay(margin money.Micros, price *money.Micros, copper *store.Material) {
	p.CustomPct = ShortMarginPercent(margin)
	if copper == nil {
		return
	}
	if price != nil {
		p.CopperPrice = CopperPriceText(*price)
	} else if price, err := pricing.PriceFromMargin(copper.PriceMicros, margin); err == nil {
		p.CopperPrice = CopperPriceText(price)
	}
}

// CCSFamilyName is the product family whose custom margin can also be typed as the
// copper price per kg.
const CCSFamilyName = "CCS & AC"

// CustomMarginValue is the margin dropdown's value for "Otro margen".
const CustomMarginValue = "custom"

// CustomMarginName is the name a custom margin carries in snapshots and pricing inputs.
// Quotes issued before issue #28 carry the earlier "Personalizado".
const CustomMarginName = "Otro margen"

// CustomOptionLabel is the dropdown's "Otro margen" entry, with its percentage once the
// draft is on one: "Otro margen (19.8%)", to two decimals.
func (p MarginPicker) CustomOptionLabel() string {
	if p.Custom && p.CustomPct != "" && p.Error == "" {
		return CustomMarginName + " (" + p.CustomPct + "%)"
	}
	return CustomMarginName
}

// CopperPriceText renders a sale price per kg for the copper field: whole centavos, no
// currency formatting, so it round-trips through an <input>.
func CopperPriceText(m money.Micros) string {
	return money.Micros(money.RoundHalfUp(int64(m), 10_000) * 10_000).String()
}

func marginChoiceValue(id int64) string {
	return strconv.FormatInt(id, 10)
}

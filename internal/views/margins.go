package views

import (
	"strconv"

	"github.com/gerrygoo/cladex-web/internal/money"
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
		p.Choices = append(p.Choices, MarginChoice{ID: o.ID, Label: label, Selected: selected})
		found = found || selected
	}
	p.NoneChosen = !found
	return p
}

func marginChoiceValue(id int64) string {
	return strconv.FormatInt(id, 10)
}

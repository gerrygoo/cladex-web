package views

import (
	"strconv"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web/internal/money"
	"github.com/gerrygoo/cladex-web/internal/store"
)

func projectFilePath(folio string, fileID int64) string {
	return projectPath(folio) + "/archivos/" + strconv.FormatInt(fileID, 10)
}

// paymentMethodText is a forma de pago with what its initials stand for.
func paymentMethodText(method string) string {
	switch method {
	case "PUE":
		return "P.U.E. · pago en una sola exhibición"
	case "PPD":
		return "P.P.D. · pago en parcialidades o diferido"
	}
	return method
}

// invoiceLabel names the factura on record by what it is for the order's forma de pago,
// and invoiceWord is the same in running text.
func invoiceLabel(method string) string {
	if method == "PPD" {
		return "Factura de anticipo"
	}
	return "Factura"
}

func invoiceWord(method string) string {
	if method == "PPD" {
		return "factura de anticipo"
	}
	return "factura"
}

// paymentBadge is the CSS suffix for a payment state's badge.
func paymentBadge(status string) string {
	if status == "" {
		return "sin_tramitar"
	}
	return status
}

// ocDateValue is the purchase order form's date: the one on record, or today for a
// proyecto that has none yet.
func ocDateValue(p store.Project, today string) string {
	if p.OC.Date != "" {
		return p.OC.Date
	}
	return today
}

// fileSizeText is a file size as people read it: "84 KB", "2.3 MB".
func fileSizeText(size int64) string {
	if size < 1<<20 {
		return strconv.FormatInt(max(1, (size+512)>>10), 10) + " KB"
	}
	return strconv.FormatFloat(float64(size)/(1<<20), 'f', 1, 64) + " MB"
}

// joinNonEmpty joins the parts that have something in them.
func joinNonEmpty(sep string, parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

// Forecast is the Pronóstico page's data: the prospectos shown, whether that is every
// prospecto or only the ones relevante para pronóstico, and the viewer's today.
type Forecast struct {
	Projects []store.Project
	All      bool
	Today    string
}

// ForecastTotals is what the listed proyectos add up to before IVA: cost, price and
// profit, and the margin that profit is of the price; and the expected amount, each
// total (with IVA, as in the proyectos list) times its probability.
type ForecastTotals struct {
	Cost, Price, Profit, Weighted money.Centavos
	Margin                        string
}

// Totals adds up the listed proyectos. The margin is the sum of profits over the sum of
// prices, not an average of their margins.
func (f Forecast) Totals() ForecastTotals {
	var price, profit, weight int64
	for _, p := range f.Projects {
		price += int64(p.Subtotal)
		profit += int64(p.Profit)
		weight += p.Weight()
	}
	return ForecastTotals{
		Cost: money.Centavos(price - profit), Price: money.Centavos(price), Profit: money.Centavos(profit),
		Weighted: store.WeightedTotal(weight), Margin: marginPercentText(profit, price),
	}
}

// marginPercentText is profit as a percentage of price with one decimal ("25.0%"), or "—"
// when there is no price to be a margin of.
func marginPercentText(profit, price int64) string {
	if price == 0 {
		return "—"
	}
	return strconv.FormatFloat(float64(profit)*100/float64(price), 'f', 1, 64) + "%"
}

// ProjectMargin is a proyecto's margin on its current quote, see marginPercentText.
func ProjectMargin(p store.Project) string {
	return marginPercentText(int64(p.Profit), int64(p.Subtotal))
}

// daysBetween is the whole days from day a to day b (both YYYY-MM-DD; an ISO timestamp's
// date part works too), negative when b is before a. ok is false when either isn't a day.
func daysBetween(a, b string) (days int, ok bool) {
	if len(a) > 10 {
		a = a[:10]
	}
	if len(b) > 10 {
		b = b[:10]
	}
	ta, errA := time.Parse("2006-01-02", a)
	tb, errB := time.Parse("2006-01-02", b)
	if errA != nil || errB != nil {
		return 0, false
	}
	return int(tb.Sub(ta).Hours() / 24), true
}

// DaysWithoutUpdate is how many days ago the proyecto last had news.
func DaysWithoutUpdate(p store.Project, today string) string {
	if d, ok := daysBetween(p.LastMovementAt, today); ok {
		return strconv.Itoa(max(d, 0))
	}
	return ""
}

// vigenciaWarnDays is how close to its end a quote's vigencia is flagged.
const vigenciaWarnDays = 5

// FollowUpWarning says what is wrong with a prospecto's next follow-up: "" when it is set
// and still to come, "Sin fecha" when there is none, "Vencido" when it is today or past.
// The table marks the cell with the words, not only the colour.
func FollowUpWarning(p store.Project, today string) string {
	switch {
	case p.NextFollowUpDate == "":
		return "Sin fecha"
	case p.FollowUpDue(today):
		return "Vencido"
	}
	return ""
}

// OCWarning is "Vencida" when the expected purchase order day has passed.
func OCWarning(p store.Project, today string) string {
	if p.ExpectedOCDate != "" && p.ExpectedOCDate < today {
		return "Vencida"
	}
	return ""
}

// VigenciaWarning says that the quote's vigencia ran out ("Vencida") or runs out within
// vigenciaWarnDays ("Vence hoy", "Vence en 3 días"). A draft has no vigencia.
func VigenciaWarning(p store.Project, today string) string {
	d, ok := daysBetween(today, p.QuoteValidUntil)
	switch {
	case !ok || d > vigenciaWarnDays:
		return ""
	case d < 0:
		return "Vencida"
	case d == 0:
		return "Vence hoy"
	case d == 1:
		return "Vence mañana"
	}
	return "Vence en " + strconv.Itoa(d) + " días"
}

// nextDay is the day after day (YYYY-MM-DD), the earliest a next follow-up can be set.
// It returns "" for something that isn't a day.
func nextDay(day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}

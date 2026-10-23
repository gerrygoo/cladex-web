package views

import (
	"net/url"
	"strings"
	"unicode"

	"github.com/gerrygoo/cladex-web/internal/store"
)

// maxPDFNameCustomerRunes caps the customer part of a quote's PDF file name.
const maxPDFNameCustomerRunes = 80

// QuotePDFName is the file name a quote's PDF downloads as, <folio>_<cliente>.pdf
// (issue #45): the quote's own folio, revision suffix included, and the customer as
// printed on it, which for an issued quote is the issue-time snapshot. Just
// <folio>.pdf when the name has nothing usable left.
func QuotePDFName(quote store.Quote) string {
	customer := quote.CustomerName
	if quote.CustomerNameSnapshot != nil {
		customer = *quote.CustomerNameSnapshot
	}
	if customer = pdfNamePart(customer); customer == "" {
		return quote.Folio + ".pdf"
	}
	return quote.Folio + "_" + customer + ".pdf"
}

// QuotePDFPath is the quote's PDF URL, ending in its file name: iOS names a PDF opened
// in a tab after the URL's last segment (not Content-Disposition), so sharing it to
// WhatsApp would otherwise send "Safari.pdf". The name segment is only cosmetic; the
// folio decides which PDF is served.
func QuotePDFPath(quote store.Quote) string {
	return "/cotizaciones/" + url.PathEscape(quote.Folio) + "/pdf/" + url.PathEscape(QuotePDFName(quote))
}

// pdfNamePart keeps a name as stored (accents, spaces, case) minus what file systems
// reject or mangle: path separators, : * ? " < > | and control characters. Whitespace
// runs collapse to one space, the result is capped at maxPDFNameCustomerRunes, and
// trailing dots and spaces (dropped by Windows) are trimmed.
func pdfNamePart(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxPDFNameCustomerRunes {
		s = string(r[:maxPDFNameCustomerRunes])
	}
	return strings.TrimRight(s, ". ")
}

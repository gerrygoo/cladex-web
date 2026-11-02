package notify

import (
	"bytes"
	"html/template"
	"strings"
)

// emailHTML wraps a message's paragraphs in a plain, readable email.
var emailHTML = template.Must(template.New("email").Parse(`<!doctype html>
<html lang="es"><body style="font-family: Arial, Helvetica, sans-serif; font-size: 15px; line-height: 1.5; color: #1f2933; max-width: 600px;">
{{range .}}<p>{{.}}</p>
{{end}}</body></html>
`))

// Email renders paragraphs as an email's text and HTML bodies.
func Email(paragraphs ...string) (text, html string) {
	var b bytes.Buffer
	if err := emailHTML.Execute(&b, paragraphs); err != nil {
		panic(err) // the template and its input are both ours
	}
	return strings.Join(paragraphs, "\n\n") + "\n", b.String()
}

// PruebaKind is the notification "Enviar prueba" sends.
const PruebaKind = "prueba"

// Prueba is the test message an admin sends to themselves from Notificaciones.
func Prueba(name string) (subject, text, html string) {
	text, html = Email(
		"Hola, "+name+".",
		"Este es un correo de prueba enviado desde Configuración del sistema → Notificaciones. "+
			"Si lo recibiste, los avisos por correo funcionan.",
		"— Cladex",
	)
	return "Prueba de avisos de Cladex", text, html
}

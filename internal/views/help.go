package views

// HelpTopic is a user-guide URL (/ayuda/<página>#<sección>) that a screen's "?" link
// opens. Every topic a view uses must be one of these constants and listed in
// HelpTopics, where a test checks it against the rendered guide — so renaming a guide
// heading without updating the link fails the build instead of landing on the top of
// the page.
type HelpTopic string

const (
	HelpInicio             HelpTopic = "/ayuda"
	HelpMiCuenta           HelpTopic = "/ayuda/acceso#cambiar-tu-contraseña"
	HelpClientes           HelpTopic = "/ayuda/clientes"
	HelpClienteNuevo       HelpTopic = "/ayuda/clientes#dar-de-alta-un-cliente"
	HelpClienteEditar      HelpTopic = "/ayuda/clientes#editar-un-cliente"
	HelpCotizaciones       HelpTopic = "/ayuda/cotizaciones"
	HelpCotizacionNueva    HelpTopic = "/ayuda/cotizaciones#crear-una-cotización"
	HelpCotizacionBorrador HelpTopic = "/ayuda/cotizaciones#agregar-productos-por-sku"
	HelpCotizacionEmitida  HelpTopic = "/ayuda/cotizaciones#descargar-y-enviar-el-pdf"
	HelpCotizacionRevisada HelpTopic = "/ayuda/cotizaciones#revisar-una-cotización-emitida"
	HelpProductos          HelpTopic = "/ayuda/productos"
	HelpProductoNuevo      HelpTopic = "/ayuda/productos#dar-de-alta-un-producto"
	HelpProductoEditar     HelpTopic = "/ayuda/productos#editar-un-producto"
	HelpConversiones       HelpTopic = "/ayuda/productos#conversiones-de-unidades"

	// Admin-only screens link to the admin-only guide page.
	HelpAjustes  HelpTopic = "/ayuda/administracion#actualizar-tipo-de-cambio-precio-del-cobre-y-margen"
	HelpUsuarios HelpTopic = "/ayuda/administracion#cambiar-el-rol-de-un-usuario"
	HelpUnidades HelpTopic = "/ayuda/administracion#unidades"
)

// HelpTopics lists every topic with whether it is only linked from admin screens.
var HelpTopics = map[HelpTopic]bool{
	HelpInicio:             false,
	HelpMiCuenta:           false,
	HelpClientes:           false,
	HelpClienteNuevo:       false,
	HelpClienteEditar:      false,
	HelpCotizaciones:       false,
	HelpCotizacionNueva:    false,
	HelpCotizacionBorrador: false,
	HelpCotizacionEmitida:  false,
	HelpCotizacionRevisada: false,
	HelpProductos:          false,
	HelpProductoNuevo:      false,
	HelpProductoEditar:     false,
	HelpConversiones:       false,
	HelpAjustes:            true,
	HelpUsuarios:           true,
	HelpUnidades:           true,
}

func quoteHelpTopic(status string) HelpTopic {
	switch status {
	case "emitida":
		return HelpCotizacionEmitida
	case "revisada":
		return HelpCotizacionRevisada
	}
	return HelpCotizacionBorrador
}

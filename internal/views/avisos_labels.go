package views

import (
	"github.com/gerrygoo/cladex-web/internal/avisos"
	"github.com/gerrygoo/cladex-web/internal/store"
)

// The avisos' names on Notificaciones and Trabajos.
func init() {
	for _, k := range store.AvisoKinds {
		notificationKindLabels[k.Key] = k.Label
	}
	jobKindLabels[avisos.JobKind] = "Ronda de avisos de seguimiento"
	jobKindLabels[avisos.OCJobKind] = "Avisos de O.C. del cliente recibida"
	jobKindLabels[avisos.SupplierJobKind] = "Recordatorio de O.C. al proveedor sin enviar"
	jobKindLabels[avisos.PerdidosJobKind] = "Resumen mensual de proyectos perdidos"
}

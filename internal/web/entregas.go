package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gerrygoo/cladex-web/internal/store"
	"github.com/gerrygoo/cladex-web/internal/views"
)

// Estado de entrega (#66): marking a supplier O.C. delivered, adding its comprobante
// later, taking it back to En proceso de entrega and cancelling it. They live under the
// O.C. (/proyectos/{folio}/ordenes-de-compra/{id}/…) and act on a confirmed one.

// projectEntrega builds the proyecto page's Estado de entrega and Entrega section from
// its O.C.s: the confirmed ones, with their delivery in force.
func projectEntrega(ctx context.Context, s *store.Store, user *store.AuthenticatedUser, project *store.Project, orders []store.PurchaseOrder) (views.ProjectEntrega, error) {
	e := views.ProjectEntrega{CanMark: canEditPO(user, project), IsAdmin: user != nil && user.Role == store.RoleAdmin}
	deliveries, err := s.ListDeliveries(ctx, project.ID)
	if err != nil {
		return e, err
	}
	bySeq := map[int64]*store.Delivery{}
	for i := range deliveries {
		bySeq[deliveries[i].Seq] = &deliveries[i]
	}
	for _, o := range orders {
		if o.Status == store.POConfirmada {
			e.Orders = append(e.Orders, views.EntregaOrder{PO: o, Delivery: bySeq[o.Seq]})
		}
	}
	return e, nil
}

// entregaRedirect goes back to the O.C.'s block in the proyecto's Entrega section.
func entregaRedirect(w http.ResponseWriter, r *http.Request, project *store.Project, po *store.PurchaseOrder) {
	http.Redirect(w, r, "/proyectos/"+project.Folio+"#entrega-"+strconv.FormatInt(po.ID, 10), http.StatusSeeOther)
}

// parseUpload reads a form that may carry a comprobante (multipart, a little over the
// 10 MB cap for its other fields). It writes the error and returns false when the body is
// too big.
func parseUpload(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxOCFileBytes+1<<20)
	if err := r.ParseMultipartForm(maxOCFileBytes + 1<<20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		http.Error(w, "el comprobante no puede pesar más de 10 MB", http.StatusRequestEntityTooLarge)
		return false
	}
	return true
}

// deliveryFile reads the comprobante de entrega from "archivo": PDF, JPG or PNG up to
// 10 MB, like the O.C.'s file. Missing, it is the message asking for it.
func deliveryFile(r *http.Request) (*store.NewFile, string, int) {
	f, msg, status := uploadedFile(r, "archivo", maxOCFileBytes, ocFileTypes,
		"el comprobante no puede pesar más de 10 MB", "el comprobante debe ser PDF, JPG o PNG", "comprobante-de-entrega")
	if msg == "" && f == nil {
		return nil, "adjunta el comprobante de entrega: PDF, JPG o PNG de hasta 10 MB", http.StatusBadRequest
	}
	return f, msg, status
}

// deliveryError answers a refused delivery write.
func deliveryError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrPODelivered):
		http.Error(w, "esta orden de compra ya está marcada como entregada", http.StatusConflict)
	case errors.Is(err, store.ErrPONotDelivered):
		http.Error(w, "esta orden de compra no está marcada como entregada", http.StatusConflict)
	case errors.Is(err, store.ErrDeliveryHasFile):
		http.Error(w, "esta entrega ya tiene su comprobante", http.StatusConflict)
	case errors.Is(err, store.ErrBadTransition):
		http.Error(w, "la entrega solo se marca en una orden de compra confirmada de un proyecto En curso", http.StatusConflict)
	default:
		serverError(w, r, err)
	}
}

// Entregar handles POST .../{id}/entregar: "Marcar como entregado" on a confirmed O.C. of
// an en curso proyecto. "fecha" is the day it was delivered (required, not after the
// viewer's today) and "comprobante" sí or no; with sí, "archivo" (PDF, JPG or PNG up to
// 10 MB) is required. The proyecto's vendedor and the admins can.
func (h *PurchaseOrders) Entregar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project, po := h.loadPO(w, r)
	if po == nil {
		return
	}
	user, _ := UserFromContext(ctx)
	if !canEditPO(user, project) {
		http.Error(w, "solo el vendedor del proyecto o un administrador marca la entrega", http.StatusForbidden)
		return
	}
	if !parseUpload(w, r) {
		return
	}
	day := strings.TrimSpace(r.FormValue("fecha"))
	if _, err := time.Parse("2006-01-02", day); err != nil {
		http.Error(w, "escribe la fecha de entrega", http.StatusBadRequest)
		return
	}
	if day > today(r) {
		http.Error(w, "la fecha de entrega no puede ser futura", http.StatusBadRequest)
		return
	}
	var file *store.NewFile
	switch r.FormValue("comprobante") {
	case "si":
		f, msg, status := deliveryFile(r)
		if msg != "" {
			http.Error(w, msg, status)
			return
		}
		file = f
	case "no", "":
	default:
		http.Error(w, "elige si quieres subir un comprobante de entrega", http.StatusBadRequest)
		return
	}
	if err := h.store.MarkDelivered(ctx, po.ID, user.ID, day, file); err != nil {
		deliveryError(w, r, err)
		return
	}
	entregaRedirect(w, r, project, po)
}

// ComprobanteEntrega handles POST .../{id}/comprobante-entrega: adds the comprobante de
// entrega ("archivo", required) to an O.C. marked delivered without one. The proyecto's
// vendedor and the admins can.
func (h *PurchaseOrders) ComprobanteEntrega(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project, po := h.loadPO(w, r)
	if po == nil {
		return
	}
	user, _ := UserFromContext(ctx)
	if !canEditPO(user, project) {
		http.Error(w, "solo el vendedor del proyecto o un administrador sube el comprobante de entrega", http.StatusForbidden)
		return
	}
	if !parseUpload(w, r) {
		return
	}
	file, msg, status := deliveryFile(r)
	if msg != "" {
		http.Error(w, msg, status)
		return
	}
	if err := h.store.AddDeliveryFile(ctx, po.ID, user.ID, file); err != nil {
		deliveryError(w, r, err)
		return
	}
	entregaRedirect(w, r, project, po)
}

// motivo reads the required "reason" or "motivo" of an admin's action.
func motivo(w http.ResponseWriter, r *http.Request, field string) (string, bool) {
	reason := strings.TrimSpace(r.FormValue(field))
	if reason == "" || utf8.RuneCountInString(reason) > maxCommentLen {
		http.Error(w, "escribe el motivo, de no más de 2000 caracteres", http.StatusBadRequest)
		return "", false
	}
	return reason, true
}

// DeshacerEntrega handles POST .../{id}/deshacer-entrega (admin): takes an O.C. marked
// delivered by mistake back to En proceso de entrega, with a required "reason" that goes
// into the history.
func (h *PurchaseOrders) DeshacerEntrega(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project, po := h.loadPO(w, r)
	if po == nil {
		return
	}
	reason, ok := motivo(w, r, "reason")
	if !ok {
		return
	}
	user, _ := UserFromContext(ctx)
	if err := h.store.UndoDelivery(ctx, po.ID, user.ID, reason); err != nil {
		deliveryError(w, r, err)
		return
	}
	entregaRedirect(w, r, project, po)
}

// Cancelar handles POST .../{id}/cancelar (admin): cancels a confirmed O.C. that isn't
// delivered, with a required "motivo" that goes into the history.
func (h *PurchaseOrders) Cancelar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project, po := h.loadPO(w, r)
	if po == nil {
		return
	}
	reason, ok := motivo(w, r, "motivo")
	if !ok {
		return
	}
	user, _ := UserFromContext(ctx)
	if err := h.store.CancelPurchaseOrder(ctx, po.ID, user.ID, reason); err != nil {
		switch {
		case errors.Is(err, store.ErrPODelivered):
			http.Error(w, "no se puede cancelar: esta orden de compra ya está entregada", http.StatusConflict)
		case errors.Is(err, store.ErrBadTransition):
			http.Error(w, "solo una orden de compra confirmada se puede cancelar", http.StatusConflict)
		default:
			serverError(w, r, err)
		}
		return
	}
	http.Redirect(w, r, views.POPath(project.Folio, po.ID, "")+"?cancelada=1", http.StatusSeeOther)
}

package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// A lead is created, followed up, lost and reopened, and converted: a new customer
// from its company, a draft quote for its vendedor, and once issued, the proyecto
// knows where it came from (#82, slice L2).
func TestLeadLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	customerID, userID, _, _ := seedQuoteFixtures(t, s, ctx)
	adminID, err := s.CreateUser(ctx, "ana", "Ana", "hash", "admin")
	if err != nil {
		t.Fatal(err)
	}

	if errs := (LeadInput{ContactName: " ", UserID: userID}).Errors(); errs["contact_name"] == "" || errs["phone"] == "" {
		t.Errorf("empty lead errors = %v", errs)
	}
	if errs := (LeadInput{ContactName: "Juan", Email: "juan@", UserID: userID}).Errors(); errs["email"] == "" {
		t.Errorf("bad email errors = %v", errs)
	}

	id, err := s.CreateLead(ctx, LeadInput{ContactName: "Juan Pérez", Company: "Eléctrica Norte", Phone: "5512345678",
		UserID: userID, NextFollowUp: "2026-11-20"}, "Pidió precios de CCA.", adminID)
	if err != nil {
		t.Fatal(err)
	}
	l, err := s.LeadByID(ctx, id)
	if err != nil || l == nil || l.Label() != "Juan Pérez · Eléctrica Norte" || !l.Open() || l.LastBody != "Pidió precios de CCA." {
		t.Fatalf("new lead = %+v, %v", l, err)
	}
	if !l.FollowUpDue("2026-11-20") || l.FollowUpDue("2026-11-19") {
		t.Error("FollowUpDue is off")
	}

	// A new contact of an existing customer.
	other, err := s.CreateLead(ctx, LeadInput{ContactName: "Carla", CustomerID: &customerID, Company: "ignorada", Email: "carla@peme.mx", UserID: adminID}, "", adminID)
	if err != nil {
		t.Fatal(err)
	}
	if o, _ := s.LeadByID(ctx, other); o.Company != "" || o.CompanyName() == "" || o.CustomerID == nil {
		t.Errorf("customer lead = %+v", o)
	}
	if mine, _ := s.ListLeads(ctx, LeadAbierto, userID); len(mine) != 1 || mine[0].ID != id {
		t.Errorf("ListLeads(mine) = %+v", mine)
	}
	if all, _ := s.ListLeads(ctx, "", 0); len(all) != 2 || all[0].ID != id {
		t.Errorf("ListLeads(all) = %d leads, first %+v; want the one with a date first", len(all), all)
	}
	if due, _ := s.LeadsFollowUpBefore(ctx, "2026-11-21"); len(due) != 1 || due[0].ID != id {
		t.Errorf("LeadsFollowUpBefore = %+v", due)
	}

	next := "2026-11-25"
	if err := s.FollowUpLead(ctx, id, userID, &next, "Le mandé la lista."); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateLead(ctx, id, LeadInput{ContactName: "Juan Pérez", Company: "Eléctrica Norte", Phone: "5512345678", UserID: adminID}, adminID); err != nil {
		t.Fatal(err)
	}
	comments, _ := s.ListLeadComments(ctx, id)
	if len(comments) != 3 || comments[0].Body != "Vendedor responsable: Vendedor Uno → Ana." ||
		comments[1].Body != "Próximo seguimiento: 25/11/2026.\nLe mandé la lista." {
		var bodies []string
		for _, c := range comments {
			bodies = append(bodies, c.Body)
		}
		t.Errorf("history = %q", bodies)
	}

	if err := s.LoseLead(ctx, id, adminID, "Compró en otro lado"); err != nil {
		t.Fatal(err)
	}
	if err := s.FollowUpLead(ctx, id, adminID, nil, "x"); !errors.Is(err, ErrLeadState) {
		t.Errorf("follow-up on a lost lead: %v", err)
	}
	if l, _ = s.LeadByID(ctx, id); l.Status != LeadPerdido || l.NextFollowUp != "" || l.LostReason != "Compró en otro lado" {
		t.Errorf("lost lead = %+v", l)
	}
	if err := s.ReopenLead(ctx, id, adminID); err != nil {
		t.Fatal(err)
	}

	qaID, _ := s.QuoteTypeIDByPrefix(ctx, "QA")
	q, err := s.ConvertLead(ctx, id, userID, qaID)
	if err != nil {
		t.Fatal(err)
	}
	if q.UserID != adminID || q.CustomerName != "Eléctrica Norte" || !strings.HasPrefix(q.Folio, "QA") {
		t.Errorf("converted quote = %+v; want Ana's, for the new customer Eléctrica Norte", q)
	}
	c, _ := s.CustomerByID(ctx, q.CustomerID)
	if c.ContactName != "Juan Pérez" || c.Phone != "5512345678" {
		t.Errorf("new customer = %+v", c)
	}
	if l, _ = s.LeadByID(ctx, id); l.Status != LeadConvertido || l.QuoteFolio != q.Folio || l.CustomerID == nil || *l.CustomerID != q.CustomerID {
		t.Errorf("converted lead = %+v", l)
	}
	if _, err := s.ConvertLead(ctx, id, userID, qaID); !errors.Is(err, ErrLeadState) {
		t.Errorf("converting twice: %v", err)
	}

	if err := s.IssueQuote(ctx, q.ID, testIssue("sha")); err != nil {
		t.Fatal(err)
	}
	q, _ = s.QuoteByID(ctx, q.ID)
	from, err := s.LeadOfProject(ctx, *q.ProjectID)
	if err != nil || from == nil || from.ID != id || from.ProjectFolio == "" {
		t.Errorf("LeadOfProject = %+v, %v", from, err)
	}

	// A lead of an existing customer converts onto that customer.
	q2, err := s.ConvertLead(ctx, other, adminID, qaID)
	if err != nil || q2.CustomerID != customerID {
		t.Errorf("converted customer lead = %+v, %v", q2, err)
	}
}

// Deleting the draft a lead was converted into opens the lead again.
func TestDeleteConvertedDraftReopensLead(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, userID, _, _ := seedQuoteFixtures(t, s, ctx)
	id, err := s.CreateLead(ctx, LeadInput{ContactName: "Juan", Phone: "55", UserID: userID}, "", userID)
	if err != nil {
		t.Fatal(err)
	}
	qaID, _ := s.QuoteTypeIDByPrefix(ctx, "QA")
	q, err := s.ConvertLead(ctx, id, userID, qaID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteDraftQuote(ctx, q.ID); err != nil {
		t.Fatalf("DeleteDraftQuote: %v", err)
	}
	if l, _ := s.LeadByID(ctx, id); l.Status != LeadAbierto || l.QuoteID != nil {
		t.Errorf("lead after deleting its draft = %+v", l)
	}
}

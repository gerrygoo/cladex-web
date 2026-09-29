package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPaginateClampsPageAndSize(t *testing.T) {
	rows := make([]int, 45)
	tests := []struct {
		query           string
		wantLen, wantPg int
		wantPer, wantOf int
	}{
		{"", 20, 1, 20, 3},
		{"page=3", 5, 3, 20, 3},
		{"page=99", 5, 3, 20, 3},
		{"page=-2", 20, 1, 20, 3},
		{"page=abc", 20, 1, 20, 3},
		{"per=10&page=2", 10, 2, 10, 5},
		{"per=100", 45, 1, 100, 1},
		{"per=7", 20, 1, 20, 3}, // not one of the offered sizes
	}
	for _, tt := range tests {
		got, pg := paginate(httptest.NewRequest("GET", "/x?"+tt.query, nil), rows)
		if len(got) != tt.wantLen || pg.Page != tt.wantPg || pg.PerPage != tt.wantPer || pg.Pages != tt.wantOf || pg.Total != 45 {
			t.Errorf("%q: len=%d %+v", tt.query, len(got), pg)
		}
	}
	if got, pg := paginate(httptest.NewRequest("GET", "/x", nil), []int(nil)); len(got) != 0 || pg.Pages != 1 {
		t.Errorf("empty: %v %+v", got, pg)
	}
}

func TestCustomersListIsPaged(t *testing.T) {
	a := newTestAuth(t)
	cs := newTestCustomers(t, a)
	userID := createTestUser(t, a, "vendedor1", "vendedor", "hunter2")
	for i := 1; i <= 45; i++ {
		if _, err := a.store.CreateCustomer(context.Background(), storeCustomer(fmt.Sprintf("Cliente %02d", i))); err != nil {
			t.Fatal(err)
		}
	}
	get := func(target string, htmx bool) string {
		req := httptest.NewRequest("GET", target, nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: mustSessionToken(t, a, userID)})
		if htmx {
			req.Header.Set("HX-Request", "true")
		}
		rec := httptest.NewRecorder()
		a.RequireAuth(http.HandlerFunc(cs.List)).ServeHTTP(rec, req)
		return rec.Body.String()
	}

	body := get("/clientes?sort=name&dir=asc", false)
	if !strings.Contains(body, "Cliente 20") || strings.Contains(body, "Cliente 21") || !strings.Contains(body, "Página 1 de 3") {
		t.Error("page 1 should hold Cliente 01-20 of 3 pages")
	}
	if !strings.Contains(body, "sort=name") || !strings.Contains(body, "page=2") {
		t.Error("next link should keep the sort and point at page 2")
	}
	body = get("/clientes?sort=name&dir=asc&page=3&per=10", false)
	if !strings.Contains(body, "Cliente 21") || !strings.Contains(body, "Cliente 30") || strings.Contains(body, "Cliente 31") || !strings.Contains(body, "Página 3 de 5") {
		t.Error("per=10 page 3 should hold Cliente 21-30 of 5 pages")
	}
	if !strings.Contains(body, "per=10") {
		t.Error("page links should keep the chosen size")
	}
	frag := get("/clientes?sort=name&dir=asc", true)
	if !strings.Contains(frag, `id="pager"`) || !strings.Contains(frag, `hx-swap-oob="true"`) {
		t.Error("htmx response should carry the pager as an out-of-band swap")
	}
}

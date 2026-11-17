package web

import (
	"bytes"
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gerrygoo/cladex-web"
	"github.com/gerrygoo/cladex-web/internal/guia"
	"github.com/gerrygoo/cladex-web/internal/views"
)

func TestStaticHandler(t *testing.T) {
	js := strings.Repeat("console.log('hola');\n", 200)
	h, err := newStaticHandler(fstest.MapFS{
		"app.js":    {Data: []byte(js)},
		"logo.png":  {Data: []byte("\x89PNG\r\n\x1a\nnot really")},
		"dir/a.css": {Data: []byte(strings.Repeat("a{color:red}", 100))},
	})
	if err != nil {
		t.Fatal(err)
	}
	get := func(p string, hdr map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/"+p, nil)
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	plain := get("app.js", nil)
	if plain.Code != 200 || plain.Body.String() != js {
		t.Fatalf("plain: %d, %d bytes", plain.Code, plain.Body.Len())
	}
	if plain.Header().Get("Content-Encoding") != "" || plain.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("plain headers: %v", plain.Header())
	}
	if ct := plain.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("content type %q", ct)
	}

	gz := get("app.js", map[string]string{"Accept-Encoding": "br, gzip;q=0.8"})
	if gz.Header().Get("Content-Encoding") != "gzip" || gz.Body.Len() >= len(js) {
		t.Errorf("gzip: %v, %d bytes", gz.Header(), gz.Body.Len())
	}
	if gz.Header().Get("ETag") == plain.Header().Get("ETag") {
		t.Error("gzip and plain share an ETag")
	}
	if get("app.js", map[string]string{"Accept-Encoding": "gzip;q=0"}).Header().Get("Content-Encoding") != "" {
		t.Error("gzip served despite q=0")
	}

	for _, w := range []*httptest.ResponseRecorder{plain, gz} {
		hdr := map[string]string{"If-None-Match": w.Header().Get("ETag")}
		if w.Header().Get("Content-Encoding") == "gzip" {
			hdr["Accept-Encoding"] = "gzip"
		}
		if got := get("app.js", hdr); got.Code != http.StatusNotModified {
			t.Errorf("revalidation of %s: %d", w.Header().Get("ETag"), got.Code)
		}
	}

	if png := get("logo.png", map[string]string{"Accept-Encoding": "gzip"}); png.Header().Get("Content-Encoding") != "" {
		t.Error("png was compressed")
	}
	if get("dir/a.css", nil).Code != 200 {
		t.Error("nested file missing")
	}
	for _, p := range []string{"missing.js", "dir", ""} {
		if c := get(p, nil).Code; c != http.StatusNotFound {
			t.Errorf("%q: %d", p, c)
		}
	}
}

// TestStaticVersionedURLs pins issue #80: every asset a page links to carries its content
// tag, the handler lets the browser keep that exact URL for a year, and the current file
// still answers an old or missing tag, uncached.
func TestStaticVersionedURLs(t *testing.T) {
	staticFS, err := fs.Sub(cladex.StaticFS, "static")
	if err != nil {
		t.Fatal(err)
	}
	h, err := newStaticHandler(staticFS)
	if err != nil {
		t.Fatal(err)
	}
	var page bytes.Buffer
	if err := views.Layout("Prueba", &views.NavUser{Username: "ana"}).Render(context.Background(), &page); err != nil {
		t.Fatal(err)
	}
	urls := regexp.MustCompile(`"/static/([^"?]+)(\?v=[0-9a-f]{16})?"`).FindAllStringSubmatch(page.String(), -1)
	if len(urls) < 8 {
		t.Fatalf("found %d static URLs in the layout", len(urls))
	}
	for _, m := range urls {
		if m[2] == "" {
			t.Errorf("/static/%s has no ?v= tag", m[1])
			continue
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/"+m[1]+m[2], nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Errorf("%s%s: %d, Cache-Control %q", m[1], m[2], w.Code, w.Header().Get("Cache-Control"))
		}
	}
	for _, p := range []string{"app.css", "app.css?v=0000000000000000"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/"+p, nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: %d, Cache-Control %q", p, w.Code, w.Header().Get("Cache-Control"))
		}
	}
	if views.Asset("no-existe.js") != "/static/no-existe.js" {
		t.Error("an unknown asset got a tag")
	}
}

// TestUnstyledPageNotice pins the layout's notice for a page whose app.css didn't load
// (issues #79, #80): it's in every page, app.css hides it, and its guide link resolves.
func TestUnstyledPageNotice(t *testing.T) {
	var page bytes.Buffer
	if err := views.Layout("Prueba", nil).Render(context.Background(), &page); err != nil {
		t.Fatal(err)
	}
	const anchor = "la-página-se-ve-sin-diseño"
	if !strings.Contains(page.String(), `<p class="css-check">`) || !strings.Contains(page.String(), "/ayuda/acceso#"+anchor) {
		t.Error("the layout lacks the unstyled-page notice or its guide link")
	}
	css, err := fs.ReadFile(cladex.StaticFS, "static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`p\.css-check\s*\{\s*display:\s*none;`).Match(css) {
		t.Error("app.css doesn't hide the notice")
	}
	guide, err := guia.New(os.DirFS("../../docs/guia"))
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := guide.Page("acceso", false); !ok || !strings.Contains(string(p.HTML), `id="`+anchor+`"`) {
		t.Errorf("the guide has no %q heading on acceso", anchor)
	}
}

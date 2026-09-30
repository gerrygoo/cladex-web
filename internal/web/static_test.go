package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
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

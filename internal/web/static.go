package web

import (
	"bytes"
	"compress/gzip"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gerrygoo/cladex-web"
)

// staticFile is one embedded asset, prepared once at startup: its bytes, a gzip copy
// when that is smaller, and ETags derived from the content.
type staticFile struct {
	ctype    string
	raw      []byte
	gz       []byte // nil when compression doesn't help
	etag     string
	etagGzip string
	tag      string // the content tag a versioned URL carries (?v=tag)
}

// staticHandler serves the embedded /static/ tree. The binary's files carry no
// modification time, so a browser can't revalidate them; this gives each an ETag (a hash
// of its content, so it's right on every build, including local dev) and answers
// If-None-Match with 304. Pages link the files through views.Asset, whose URLs carry the
// content tag (?v=…, issue #80): a request whose tag matches the file is cached as
// immutable for a year, since a different file gets a different URL. Any other request
// (no tag, or a page from before a deploy asking for an old one) gets the current file
// with Cache-Control: no-cache, "reuse only after checking". Text assets are also
// served gzipped.
type staticHandler struct {
	files map[string]*staticFile
}

// newStaticHandler reads every file under fsys. It reports an unreadable tree rather
// than serving a partial one.
func newStaticHandler(fsys fs.FS) (*staticHandler, error) {
	h := &staticHandler{files: map[string]*staticFile{}}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		ctype := mime.TypeByExtension(path.Ext(p))
		if ctype == "" {
			ctype = http.DetectContentType(raw)
		}
		tag := cladex.AssetTag(raw)
		f := &staticFile{ctype: ctype, raw: raw, etag: `"` + tag + `"`, tag: tag}
		if compressible(ctype) {
			var buf bytes.Buffer
			zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
			zw.Write(raw)
			zw.Close()
			if buf.Len() < len(raw) {
				f.gz = buf.Bytes()
				f.etagGzip = `"` + tag + `-gzip"`
			}
		}
		h.files[p] = f
		return nil
	})
	return h, err
}

func compressible(ctype string) bool {
	return strings.HasPrefix(ctype, "text/") || strings.Contains(ctype, "javascript") ||
		strings.Contains(ctype, "svg") || strings.Contains(ctype, "json")
}

// ServeHTTP expects the path relative to /static/ (mount it behind http.StripPrefix).
func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f, ok := h.files[strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", f.ctype)
	if r.URL.Query().Get("v") == f.tag {
		hd.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		hd.Set("Cache-Control", "no-cache")
	}
	hd.Add("Vary", "Accept-Encoding")
	body, tag := f.raw, f.etag
	if f.gz != nil && acceptsGzip(r) {
		body, tag = f.gz, f.etagGzip
		hd.Set("Content-Encoding", "gzip")
	}
	hd.Set("ETag", tag)
	// ServeContent answers If-None-Match with 304 using the ETag set above.
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
}

func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.EqualFold(name, "gzip") && strings.TrimSpace(params) != "q=0" {
			return true
		}
	}
	return false
}

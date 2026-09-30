package web

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

// staticFile is one embedded asset, prepared once at startup: its bytes, a gzip copy
// when that is smaller, and ETags derived from the content.
type staticFile struct {
	ctype    string
	raw      []byte
	gz       []byte // nil when compression doesn't help
	etag     string
	etagGzip string
}

// staticHandler serves the embedded /static/ tree. The binary's files carry no
// modification time, so a browser can't revalidate them; this gives each an ETag (a hash
// of its content, so it's right on every build, including local dev) and answers
// If-None-Match with 304. Cache-Control: no-cache means "reuse only after checking",
// because the URLs aren't fingerprinted and a deploy must show up at once. Text assets
// are also served gzipped.
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
		sum := sha256.Sum256(raw)
		tag := hex.EncodeToString(sum[:8])
		f := &staticFile{ctype: ctype, raw: raw, etag: `"` + tag + `"`}
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
	hd.Set("Cache-Control", "no-cache")
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

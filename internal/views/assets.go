package views

import (
	"io/fs"
	"sync"

	"github.com/gerrygoo/cladex-web"
)

// assetTags maps each file under static/ to the tag of its content, the same one the
// static handler uses as its ETag (web.AssetTag).
var assetTags = sync.OnceValue(func() map[string]string {
	tags := map[string]string{}
	_ = fs.WalkDir(cladex.StaticFS, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := fs.ReadFile(cladex.StaticFS, p)
		if err != nil {
			return err
		}
		tags[p[len("static/"):]] = cladex.AssetTag(raw)
		return nil
	})
	return tags
})

// Asset is the URL of static/name with its content tag (issue #80):
// /static/app.css?v=1a2b3c4d5e6f7a8b. A deploy that changes a file changes its URL, so a
// browser never reuses an old or damaged copy, and the static handler lets a browser keep
// a tagged URL without asking again. A name that isn't in static/ gets the bare URL (it
// would 404 anyway; TestAssetURLs catches the typo).
func Asset(name string) string {
	if tag, ok := assetTags()[name]; ok {
		return "/static/" + name + "?v=" + tag
	}
	return "/static/" + name
}

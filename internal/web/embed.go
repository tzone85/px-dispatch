package web

import (
	"io/fs"
	"net/http"

	webfs "github.com/tzone85/px-dispatch/web"
)

// staticFS is the embedded filesystem containing dashboard static assets.
var staticFS = webfs.Assets

// staticHandler returns an http.Handler serving the embedded static files.
func staticHandler() http.Handler {
	return http.FileServer(http.FS(mustSub(staticFS, ".")))
}

// mustSub returns an fs.FS rooted at dir within parent. Panics on error.
func mustSub(parent fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(parent, dir)
	if err != nil {
		panic("web: sub fs: " + err.Error())
	}
	return sub
}

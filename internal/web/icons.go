package web

import (
	"bytes"
	_ "embed"
	"net/http"
	"time"
)

//go:embed static/apple-touch-icon.png
var appleTouchIcon []byte

//go:embed static/favicon.png
var favicon []byte

// Explicit routes keep the rest of the embedded UI private. The existing
// QTS router exposes these same routes beneath /docExtractor as well.
func registerIconRoutes(mux *http.ServeMux) {
	for name, data := range map[string][]byte{
		"apple-touch-icon.png": appleTouchIcon,
		"favicon.png": favicon,
	} {
		mux.HandleFunc("GET /"+name, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Cache-Control", "public, max-age=86400")
			http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
		})
	}
}

package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed adminui/*
var adminFiles embed.FS

func adminUI() http.Handler {
	assets, err := fs.Sub(adminFiles, "adminui")
	if err != nil {
		panic(err)
	}
	files := http.StripPrefix("/admin/", http.FileServer(http.FS(assets)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Vite content-hashes everything under assets/, so it never changes in place.
		if strings.HasPrefix(r.URL.Path, "/admin/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}

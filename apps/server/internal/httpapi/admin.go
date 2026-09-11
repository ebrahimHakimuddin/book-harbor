package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed adminui/*
var adminFiles embed.FS

func adminUI() http.Handler {
	assets, err := fs.Sub(adminFiles, "adminui")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/admin/", http.FileServer(http.FS(assets)))
}

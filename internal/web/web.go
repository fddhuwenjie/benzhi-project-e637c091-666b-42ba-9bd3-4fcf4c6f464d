package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed static/*
var assets embed.FS

type Handler struct {
	static http.Handler
	index  []byte
}

func New() (*Handler, error) {
	root, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}
	index, err := fs.ReadFile(root, "index.html")
	if err != nil {
		return nil, err
	}
	return &Handler{static: http.FileServer(http.FS(root)), index: index}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clean := path.Clean(r.URL.Path)
	if clean == "/" || clean == "/archives" || strings.HasPrefix(clean, "/incidents/") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(h.index)
		return
	}
	if clean == "/app.css" || clean == "/app.js" {
		w.Header().Set("Cache-Control", "public, max-age=300")
		h.static.ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

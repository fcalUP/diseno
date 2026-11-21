package app

import (
	"bytes"
	"html"
	"io"
	"net/http"
	"os"
	"strings"
)

// spaHandler sirve el build de Vite desde WEB_DIR (spec §5.5). Cualquier GET
// que no sea archivo, /api/ ni /media/ recibe index.html. Sin WEB_DIR (en
// desarrollo el frontend lo sirve Vite) responde 404 en texto.
func (a *App) spaHandler() http.Handler {
	web, err := os.OpenRoot(a.Cfg.WebDir)
	if err != nil {
		a.Log.Warn("WEB_DIR no disponible; solo se sirve el API", "dir", a.Cfg.WebDir, "err", err)
		web = nil
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if web == nil || strings.HasPrefix(r.URL.Path, "/media/") {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name != "" && validKey(name) && serveFile(w, r, web, name, webCache(name)) {
			return
		}
		// Un chunk que ya no existe debe dar 404, no HTML con tipo equivocado.
		if strings.HasPrefix(name, "assets/") {
			http.NotFound(w, r)
			return
		}
		a.serveIndex(w, r, web)
	})
}

// serveIndex manda index.html; con WAF_SDK_URL le agrega el script del WAF para que
// el navegador renueve su token solo y las llamadas al API no caigan en un reto.
func (a *App) serveIndex(w http.ResponseWriter, r *http.Request, web *os.Root) {
	if a.Cfg.WafSDKURL == "" {
		if !serveFile(w, r, web, "index.html", "no-cache") {
			http.NotFound(w, r)
		}
		return
	}
	f, err := web.Open("index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	page, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "Error interno del servidor.", http.StatusInternalServerError)
		return
	}
	tag := `<script src="` + html.EscapeString(a.Cfg.WafSDKURL) + `" defer></script>`
	page = bytes.Replace(page, []byte("</head>"), []byte(tag+"</head>"), 1)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", info.ModTime(), bytes.NewReader(page))
}

func webCache(name string) string {
	switch {
	case name == "index.html":
		return "no-cache"
	case strings.HasPrefix(name, "assets/"):
		return "public, max-age=31536000, immutable"
	default:
		return "public, max-age=3600"
	}
}

// mediaHandler sirve /media/* en modo local: MEDIA_DIR y luego MEDIA_SEED_DIR.
func mediaHandler(ls *localStorage) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/media/")
		root, _, err := ls.find(key)
		if err != nil || !serveFile(w, r, root, key, "public, max-age=3600") {
			http.NotFound(w, r)
		}
	})
}

// serveFile manda un archivo normal de root; false si no existe o es carpeta.
func serveFile(w http.ResponseWriter, r *http.Request, root *os.Root, name, cacheControl string) bool {
	f, err := root.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	w.Header().Set("Cache-Control", cacheControl)
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
	return true
}

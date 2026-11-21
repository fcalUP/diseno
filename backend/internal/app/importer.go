package app

import "net/http"

// Stubs de B3 para que el paquete compile; B3 los reemplaza.

func (a *App) handleAdminImportStatus(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminImportPreview(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminImportRun(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminImportJob(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminImportMedia(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

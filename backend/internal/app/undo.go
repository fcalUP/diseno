package app

import "net/http"

// Stubs de B2 para que el paquete compile; B2 los reemplaza.

func (a *App) handleAdminAuditList(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminAuditUndo(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminAuditRedo(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminAuditBatchUndo(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminAuditBatchRedo(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

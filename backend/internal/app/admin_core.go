package app

import "net/http"

// Stubs de B2 para que el paquete compile; B2 los reemplaza.

func (a *App) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminLogout(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminMe(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminStudentsList(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminUpdateStudentData(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

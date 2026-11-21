package app

import "net/http"

// Stubs de B2 para que el paquete compile; B2 los reemplaza.

func (a *App) handleAdminStudentCreate(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminStudentsGenerate(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminStudentsBulk(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminStudentUpdate(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminStudentDelete(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminStudentPassword(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

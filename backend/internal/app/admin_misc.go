package app

import "net/http"

// Stubs de B2 para que el paquete compile; B2 los reemplaza.

func (a *App) handleAdminPurchasesList(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminPurchasesRevert(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminPurchasesRestore(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminSettingsGet(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminSettingsPatch(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminAdminsList(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminAdminsCreate(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminAdminsUpdate(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminAdminsDelete(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleAdminUpload(w http.ResponseWriter, r *http.Request, adminID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

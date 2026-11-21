package app

import "net/http"

// Stubs de B1 para que el paquete compile; B1 los reemplaza.

func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request, studentID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleSendResetCode(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleBadges(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handlePurchase(w http.ResponseWriter, r *http.Request, studentID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

func (a *App) handleLeaderboard(w http.ResponseWriter, r *http.Request, studentID int64) {
	writeError(w, http.StatusNotImplemented, "pendiente")
}

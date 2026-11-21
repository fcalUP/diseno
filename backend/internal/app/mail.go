package app

// Stub de B1: B1 lo reemplaza con el cliente de Resend.
func newResendMailer(a *App) Mailer { return noopMailer{} }

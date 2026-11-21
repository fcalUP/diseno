package app

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

func TestSecretEncryption(t *testing.T) {
	key := deriveKey("diseno-settings:", strings.Repeat("x", 32))
	enc, err := encryptSecret(key, "resend_api_key", "re_123")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(enc, "enc:v1:"))
	if !strings.HasPrefix(enc, "enc:v1:") || err != nil || len(raw) != 12+len("re_123")+16 {
		t.Fatalf("formato inesperado: %q", enc)
	}
	if plain, err := decryptSecret(key, "resend_api_key", enc); err != nil || plain != "re_123" {
		t.Fatalf("ida y vuelta: %q %v", plain, err)
	}
	if _, err := decryptSecret(deriveKey("diseno-settings:", strings.Repeat("y", 32)), "resend_api_key", enc); err == nil {
		t.Fatal("descifró con otra clave")
	}
	if _, err := decryptSecret(key, "google_service_account", enc); err == nil {
		t.Fatal("descifró bajo otro nombre de ajuste")
	}
	if again, _ := encryptSecret(key, "resend_api_key", "re_123"); again == enc {
		t.Fatal("el nonce debe cambiar en cada cifrado")
	}
}

func TestSettingsSaveAndLoad(t *testing.T) {
	a := isolatedApp(t)
	ctx := context.Background()

	s, err := a.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !s.StoreEnabled || s.ResetCodeTTLMinutes != 10 || s.ResendSource != "none" || len(s.EmailCc) != 1 {
		t.Fatalf("predeterminados: %+v", s)
	}

	save := func(patch map[string]any) error {
		tx, err := a.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err := a.SaveSettings(ctx, tx, patch); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		a.invalidateSettings()
		return nil
	}

	err = save(map[string]any{
		"store_enabled":          false,
		"email_cc":               []string{"a@up.edu.mx", "b@up.edu.mx"},
		"reset_code_ttl_minutes": float64(15), // así llega de un JSON decodificado
		"resend_api_key":         "re_secreto",
	})
	if err != nil {
		t.Fatal(err)
	}
	s, _ = a.Settings(ctx)
	if s.StoreEnabled || s.ResetCodeTTLMinutes != 15 || len(s.EmailCc) != 2 || s.ResendAPIKey != "re_secreto" || s.ResendSource != "db" {
		t.Fatalf("tras guardar: %+v", s)
	}
	var stored string
	if err := a.DB.QueryRow(ctx, "select value #>> '{}' from settings where key = 'resend_api_key'").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, "enc:v1:") || strings.Contains(stored, "re_secreto") {
		t.Fatalf("el secreto quedó sin cifrar: %q", stored)
	}

	// La copia que se entrega no comparte la lista con el caché.
	s.EmailCc[0] = "cambiado"
	if s2, _ := a.Settings(ctx); s2.EmailCc[0] != "a@up.edu.mx" {
		t.Fatal("Settings expone el slice del caché")
	}

	for _, bad := range []map[string]any{
		{"no_existe": true},
		{"store_enabled": "sí"},
		{"reset_code_ttl_minutes": 1.5},
		{"email_cc": []any{"a@b.c", 3}},
		{"resend_api_key": 123},
	} {
		if err := save(bad); err == nil {
			t.Errorf("debió rechazar %v", bad)
		}
	}

	// nil borra y vuelve al predeterminado; sin valor en la tabla se usa el env.
	a.Cfg.ResendAPIKey = "re_env"
	if err := save(map[string]any{"store_enabled": nil, "resend_api_key": nil}); err != nil {
		t.Fatal(err)
	}
	s, _ = a.Settings(ctx)
	if !s.StoreEnabled || s.ResendAPIKey != "re_env" || s.ResendSource != "env" {
		t.Fatalf("tras borrar: %+v", s)
	}
	if len(settingsKeys()) != len(settingsFields()) || len(settingsKeys()) != 18 {
		t.Fatalf("claves de ajustes: %v", settingsKeys())
	}
}

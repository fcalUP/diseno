package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Cuánto vive el caché de ajustes y niveles aunque nadie lo invalide.
const cacheTTL = time.Minute

// Settings son los ajustes ya resueltos: predeterminados de código + lo guardado en la tabla.
// Las etiquetas JSON coinciden con AdminSettings de types.ts; los secretos no se serializan.
type Settings struct {
	RegistrationEnabled     bool     `json:"registrationEnabled"`
	PasswordResetEnabled    bool     `json:"passwordResetEnabled"`
	StoreEnabled            bool     `json:"storeEnabled"`
	LeaderboardEnabled      bool     `json:"leaderboardEnabled"`
	LevelUpAnimationEnabled bool     `json:"levelUpAnimationEnabled"`
	PurchaseEmailEnabled    bool     `json:"purchaseEmailEnabled"`
	EmailFrom               string   `json:"emailFrom"`
	EmailCc                 []string `json:"emailCc"`
	StudentEmailDomain      string   `json:"studentEmailDomain"`
	PasswordPattern         string   `json:"passwordPattern"`
	PasswordHintRegister    string   `json:"passwordHintRegister"`
	PasswordHintReset       string   `json:"passwordHintReset"`
	PasswordError           string   `json:"passwordError"`
	ResetCodeTTLMinutes     int      `json:"resetCodeTtlMinutes"`
	LevelUpGifURL           string   `json:"levelUpGifUrl"`
	SpreadsheetID           string   `json:"spreadsheetId"`

	// Secretos descifrados; "" si no hay. Si la tabla no los tiene se usa el env.
	ResendAPIKey         string `json:"-"`
	GoogleServiceAccount string `json:"-"`
	ResendSource         string `json:"-"` // "db" | "env" | "none"
	GoogleSource         string `json:"-"`
}

func defaultSettings() Settings {
	return Settings{
		RegistrationEnabled:     true,
		PasswordResetEnabled:    true,
		StoreEnabled:            true,
		LeaderboardEnabled:      true,
		LevelUpAnimationEnabled: true,
		PurchaseEmailEnabled:    true,
		EmailFrom:               "onboarding@resend.dev",
		EmailCc:                 []string{"fcal@up.edu.mx"},
		StudentEmailDomain:      "up.edu.mx",
		PasswordPattern:         "^[A-Z0-9]{4}$",
		PasswordHintRegister:    "Password (2 uppercase letters and 2 numbers)",
		PasswordHintReset:       "New password (4 uppercase letters/numbers)",
		PasswordError:           "Invalid password. Use only 4 uppercase letters/numbers.",
		ResetCodeTTLMinutes:     10,
		LevelUpGifURL:           "/media/legacy/gifs/levelup.gif",
		SpreadsheetID:           "1I6pVLSBav-U7c86FLavh0tikPghLDVrWCFuru-qwQ4Y",
	}
}

// SettingField describe una clave de ajustes: nombre en la tabla, nombre en
// AdminSettings/SettingsPatch y si es secreta. Sirve para validar parches.
type SettingField struct {
	Key    string
	JSON   string
	Secret bool
}

type settingDef struct {
	SettingField
	field func(*Settings) any // puntero al campo de Settings
}

var settingDefs = []settingDef{
	{SettingField{"registration_enabled", "registrationEnabled", false}, func(s *Settings) any { return &s.RegistrationEnabled }},
	{SettingField{"password_reset_enabled", "passwordResetEnabled", false}, func(s *Settings) any { return &s.PasswordResetEnabled }},
	{SettingField{"store_enabled", "storeEnabled", false}, func(s *Settings) any { return &s.StoreEnabled }},
	{SettingField{"leaderboard_enabled", "leaderboardEnabled", false}, func(s *Settings) any { return &s.LeaderboardEnabled }},
	{SettingField{"level_up_animation_enabled", "levelUpAnimationEnabled", false}, func(s *Settings) any { return &s.LevelUpAnimationEnabled }},
	{SettingField{"purchase_email_enabled", "purchaseEmailEnabled", false}, func(s *Settings) any { return &s.PurchaseEmailEnabled }},
	{SettingField{"email_from", "emailFrom", false}, func(s *Settings) any { return &s.EmailFrom }},
	{SettingField{"email_cc", "emailCc", false}, func(s *Settings) any { return &s.EmailCc }},
	{SettingField{"student_email_domain", "studentEmailDomain", false}, func(s *Settings) any { return &s.StudentEmailDomain }},
	{SettingField{"password_pattern", "passwordPattern", false}, func(s *Settings) any { return &s.PasswordPattern }},
	{SettingField{"password_hint_register", "passwordHintRegister", false}, func(s *Settings) any { return &s.PasswordHintRegister }},
	{SettingField{"password_hint_reset", "passwordHintReset", false}, func(s *Settings) any { return &s.PasswordHintReset }},
	{SettingField{"password_error", "passwordError", false}, func(s *Settings) any { return &s.PasswordError }},
	{SettingField{"reset_code_ttl_minutes", "resetCodeTtlMinutes", false}, func(s *Settings) any { return &s.ResetCodeTTLMinutes }},
	{SettingField{"level_up_gif_url", "levelUpGifUrl", false}, func(s *Settings) any { return &s.LevelUpGifURL }},
	{SettingField{"spreadsheet_id", "spreadsheetId", false}, func(s *Settings) any { return &s.SpreadsheetID }},
	{SettingField{"resend_api_key", "resendApiKey", true}, func(s *Settings) any { return &s.ResendAPIKey }},
	{SettingField{"google_service_account", "googleServiceAccount", true}, func(s *Settings) any { return &s.GoogleServiceAccount }},
}

func settingsFields() []SettingField {
	out := make([]SettingField, len(settingDefs))
	for i, d := range settingDefs {
		out[i] = d.SettingField
	}
	return out
}

// settingsKeys devuelve las claves válidas de la tabla settings.
func settingsKeys() []string {
	out := make([]string, len(settingDefs))
	for i, d := range settingDefs {
		out[i] = d.Key
	}
	return out
}

func findSetting(key string) (settingDef, bool) {
	for _, d := range settingDefs {
		if d.Key == key {
			return d, true
		}
	}
	return settingDef{}, false
}

// Settings devuelve una copia de los ajustes (con caché).
func (a *App) Settings(ctx context.Context) (Settings, error) {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	if a.settings == nil || time.Since(a.settingsAt) > cacheTTL {
		s, err := a.loadSettings(ctx)
		if err != nil {
			return Settings{}, fmt.Errorf("leer ajustes: %w", err)
		}
		a.settings, a.settingsAt = &s, time.Now()
	}
	s := *a.settings
	s.EmailCc = slices.Clone(s.EmailCc)
	return s, nil
}

func (a *App) invalidateSettings() {
	a.settingsMu.Lock()
	a.settings = nil
	a.settingsMu.Unlock()
}

func (a *App) loadSettings(ctx context.Context) (Settings, error) {
	s := defaultSettings()
	rows, err := a.DB.Query(ctx, "select key, value from settings")
	if err != nil {
		return s, err
	}
	defer rows.Close()
	inDB := map[string]bool{}
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return s, err
		}
		def, ok := findSetting(key)
		if !ok {
			continue
		}
		if def.Secret {
			var enc, plain string
			err := json.Unmarshal(raw, &enc)
			if err == nil {
				plain, err = decryptSecret(a.settingsKey, key, enc)
			}
			if err != nil {
				// Pasa si cambió SESSION_SECRET: se ignora y se vuelve a capturar en Ajustes.
				a.Log.Warn("no se pudo descifrar un ajuste secreto", "key", key, "err", err)
				continue
			}
			*def.field(&s).(*string) = plain
			inDB[key] = plain != ""
			continue
		}
		if err := json.Unmarshal(raw, def.field(&s)); err != nil {
			a.Log.Warn("ajuste con valor inválido, se usa el predeterminado", "key", key, "err", err)
		}
	}
	if err := rows.Err(); err != nil {
		return s, err
	}
	s.ResendSource = secretSource(inDB["resend_api_key"], &s.ResendAPIKey, a.Cfg.ResendAPIKey)
	s.GoogleSource = secretSource(inDB["google_service_account"], &s.GoogleServiceAccount, a.Cfg.GoogleServiceAccount)
	return s, nil
}

func secretSource(inDB bool, val *string, env string) string {
	switch {
	case inDB:
		return "db"
	case env != "":
		*val = env
		return "env"
	default:
		return "none"
	}
}

// SaveSettings guarda un parche con claves de la tabla (snake_case). nil borra
// el valor guardado (vuelve al predeterminado); en secretos "" también borra.
// Los secretos se cifran aquí. Quien llama debe invocar invalidateSettings()
// después del commit.
func (a *App) SaveSettings(ctx context.Context, tx pgx.Tx, patch map[string]any) error {
	for key, v := range patch {
		def, ok := findSetting(key)
		if !ok {
			return fmt.Errorf("ajuste desconocido: %s", key)
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("valor inválido para %s", key)
		}
		if string(raw) == "null" || (def.Secret && string(raw) == `""`) {
			if _, err := tx.Exec(ctx, "delete from settings where key = $1", key); err != nil {
				return err
			}
			continue
		}
		if raw, err = a.storedSettingJSON(def, raw); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `insert into settings (key, value, updated_at) values ($1, $2, now())
			on conflict (key) do update set value = excluded.value, updated_at = now()`,
			key, json.RawMessage(raw)); err != nil {
			return err
		}
	}
	a.invalidateSettings()
	return nil
}

// storedSettingJSON revisa que el valor quepa en el tipo del campo (bool, texto,
// entero, lista de textos) y cifra los secretos.
func (a *App) storedSettingJSON(def settingDef, raw []byte) ([]byte, error) {
	probe := defaultSettings()
	if err := json.Unmarshal(raw, def.field(&probe)); err != nil {
		return nil, fmt.Errorf("valor inválido para %s", def.Key)
	}
	if !def.Secret {
		return raw, nil
	}
	enc, err := encryptSecret(a.settingsKey, def.Key, *def.field(&probe).(*string))
	if err != nil {
		return nil, err
	}
	return json.Marshal(enc)
}

const secretPrefix = "enc:v1:"

// encryptSecret cifra con AES-256-GCM: "enc:v1:" + base64(nonce|cifrado).
// El nombre del ajuste va como dato asociado para que no se pueda mover a otra clave.
func encryptSecret(key []byte, name, plain string) (string, error) {
	gcm, err := secretAEAD(key)
	if err != nil {
		return "", err
	}
	return secretPrefix + base64.StdEncoding.EncodeToString(gcm.Seal(nil, nil, []byte(plain), []byte(name))), nil
}

func decryptSecret(key []byte, name, enc string) (string, error) {
	b64, ok := strings.CutPrefix(enc, secretPrefix)
	if !ok {
		return "", errors.New("formato de secreto desconocido")
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	gcm, err := secretAEAD(key)
	if err != nil {
		return "", err
	}
	plain, err := gcm.Open(nil, nil, data, []byte(name))
	if err != nil {
		return "", errors.New("no se pudo descifrar")
	}
	return string(plain), nil
}

// secretAEAD usa nonce aleatorio de 12 bytes que va al inicio del cifrado.
func secretAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}

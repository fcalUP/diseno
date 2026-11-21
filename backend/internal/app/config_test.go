package app

import (
	"strings"
	"testing"
)

var configVars = []string{
	"PORT", "DATABASE_URL", "SESSION_SECRET", "PUBLIC_URL", "COOKIE_SECURE", "TRUST_PROXY_HEADERS",
	"WEB_DIR", "STORAGE_DRIVER", "MEDIA_DIR", "MEDIA_SEED_DIR", "S3_BUCKET", "S3_PREFIX", "AWS_REGION",
	"UPLOAD_ORIGIN", "ADMIN_USERNAME", "ADMIN_PASSWORD", "RESEND_API_KEY", "GOOGLE_SERVICE_ACCOUNT",
}

func setConfigEnv(t *testing.T, vals map[string]string) {
	for _, k := range configVars {
		t.Setenv(k, vals[k])
	}
}

func TestLoadConfig(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://localhost/x", "SESSION_SECRET": strings.Repeat("s", 32)}
	setConfigEnv(t, base)
	c, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != "8080" || c.PublicURL != "http://localhost:5173" || c.StorageDriver != "local" || c.S3Prefix != "media/" ||
		c.AWSRegion != "us-east-1" || c.AdminUsername != "Admin" || c.CookieSecure || c.UploadOrigin != "" {
		t.Fatalf("predeterminados: %+v", c)
	}

	setConfigEnv(t, map[string]string{"DATABASE_URL": "postgres://localhost/x", "SESSION_SECRET": strings.Repeat("s", 32),
		"STORAGE_DRIVER": "s3", "S3_BUCKET": "diseno-media", "AWS_REGION": "us-east-2", "S3_PREFIX": "m",
		"COOKIE_SECURE": "true", "PUBLIC_URL": "https://diseno.example.com/"})
	c, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.UploadOrigin != "https://diseno-media.s3.us-east-2.amazonaws.com" || c.S3Prefix != "m/" || !c.CookieSecure ||
		c.PublicURL != "https://diseno.example.com" {
		t.Fatalf("s3: %+v", c)
	}

	bad := []map[string]string{
		{"SESSION_SECRET": strings.Repeat("s", 32)},                                                                   // sin DATABASE_URL
		{"DATABASE_URL": "postgres://localhost/x", "SESSION_SECRET": "corta"},                                         // secreto corto
		{"DATABASE_URL": "postgres://localhost/x", "SESSION_SECRET": strings.Repeat("s", 32), "STORAGE_DRIVER": "s3"}, // sin bucket
		{"DATABASE_URL": "postgres://localhost/x", "SESSION_SECRET": strings.Repeat("s", 32), "STORAGE_DRIVER": "ftp"},
		{"DATABASE_URL": "postgres://localhost/x", "SESSION_SECRET": strings.Repeat("s", 32), "COOKIE_SECURE": "quizá"},
		{"DATABASE_URL": "postgres://localhost/x", "SESSION_SECRET": strings.Repeat("s", 32), "PUBLIC_URL": "diseno.com"},
		{"DATABASE_URL": "postgres://localhost/x", "SESSION_SECRET": strings.Repeat("s", 32), "PORT": "abc"},
	}
	for _, vals := range bad {
		setConfigEnv(t, vals)
		if _, err := LoadConfig(); err == nil {
			t.Errorf("debió fallar con %v", vals)
		}
	}
}

package app

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config viene de variables de entorno (spec §5.1).
type Config struct {
	Port                 string
	DatabaseURL          string
	SessionSecret        string
	PublicURL            string // sin barra final
	CookieSecure         bool
	TrustProxy           bool // TRUST_PROXY_HEADERS
	WebDir               string
	StorageDriver        string // "local" | "s3"
	MediaDir             string
	MediaSeedDir         string
	S3Bucket             string
	S3Prefix             string // termina en "/"
	AWSRegion            string
	UploadOrigin         string // origen de S3 para connect-src de la CSP
	WafSDKURL            string // challenge.js del WAF; vacío = no se inyecta
	WafSDKOrigin         string // su origen, para la CSP
	AdminUsername        string
	AdminPassword        string
	ResendAPIKey         string // respaldo si no está en Ajustes
	GoogleServiceAccount string // respaldo si no está en Ajustes
}

func LoadConfig() (Config, error) {
	c := Config{
		Port:                 envOr("PORT", "8080"),
		DatabaseURL:          envOr("DATABASE_URL", ""),
		SessionSecret:        envOr("SESSION_SECRET", ""),
		PublicURL:            strings.TrimRight(envOr("PUBLIC_URL", "http://localhost:5173"), "/"),
		WebDir:               envOr("WEB_DIR", "./web"),
		StorageDriver:        strings.ToLower(envOr("STORAGE_DRIVER", "local")),
		MediaDir:             envOr("MEDIA_DIR", "./data/media"),
		MediaSeedDir:         envOr("MEDIA_SEED_DIR", ""),
		S3Bucket:             envOr("S3_BUCKET", ""),
		S3Prefix:             envOr("S3_PREFIX", "media/"),
		AWSRegion:            envOr("AWS_REGION", "us-east-1"),
		UploadOrigin:         strings.TrimRight(envOr("UPLOAD_ORIGIN", ""), "/"),
		WafSDKURL:            envOr("WAF_SDK_URL", ""),
		AdminUsername:        envOr("ADMIN_USERNAME", "Admin"),
		AdminPassword:        os.Getenv("ADMIN_PASSWORD"), // tal cual, puede llevar espacios
		ResendAPIKey:         envOr("RESEND_API_KEY", ""),
		GoogleServiceAccount: envOr("GOOGLE_SERVICE_ACCOUNT", ""),
	}
	var errs []error
	var err error
	if c.CookieSecure, err = envBool("COOKIE_SECURE"); err != nil {
		errs = append(errs, err)
	}
	if c.TrustProxy, err = envBool("TRUST_PROXY_HEADERS"); err != nil {
		errs = append(errs, err)
	}
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL es obligatoria"))
	}
	if len(c.SessionSecret) < 32 {
		errs = append(errs, errors.New("SESSION_SECRET es obligatoria y debe tener al menos 32 caracteres"))
	}
	if p, err := strconv.Atoi(c.Port); err != nil || p < 1 || p > 65535 {
		errs = append(errs, fmt.Errorf("PORT inválido: %q", c.Port))
	}
	if u, err := url.Parse(c.PublicURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, fmt.Errorf("PUBLIC_URL inválida: %q", c.PublicURL))
	}
	if c.WafSDKURL != "" {
		if u, err := url.Parse(c.WafSDKURL); err != nil || u.Scheme != "https" || u.Host == "" {
			errs = append(errs, fmt.Errorf("WAF_SDK_URL inválida: %q", c.WafSDKURL))
		} else {
			c.WafSDKOrigin = "https://" + u.Host
		}
	}
	if !strings.HasSuffix(c.S3Prefix, "/") {
		c.S3Prefix += "/"
	}
	switch c.StorageDriver {
	case "local":
	case "s3":
		if c.S3Bucket == "" {
			errs = append(errs, errors.New("S3_BUCKET es obligatoria con STORAGE_DRIVER=s3"))
		} else if c.UploadOrigin == "" {
			c.UploadOrigin = "https://" + c.S3Bucket + ".s3." + c.AWSRegion + ".amazonaws.com"
		}
	default:
		errs = append(errs, fmt.Errorf("STORAGE_DRIVER debe ser local o s3, no %q", c.StorageDriver))
	}
	return c, errors.Join(errs...)
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envBool(key string) (bool, error) {
	v := envOr(key, "false")
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s debe ser true o false, no %q", key, v)
	}
	return b, nil
}

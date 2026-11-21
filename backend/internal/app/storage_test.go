package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMediaKey(t *testing.T) {
	if k, ok := mediaKey(mediaURL("u/2026/11/ab.png")); !ok || k != "u/2026/11/ab.png" {
		t.Fatalf("mediaKey(mediaURL(x)) = %q, %v", k, ok)
	}
	for _, bad := range []string{"", "/media/", "/img/x.png", "/media/../x", "/media//x", "/media/a/./b", "https://x/media/a"} {
		if _, ok := mediaKey(bad); ok {
			t.Errorf("mediaKey(%q) debió rechazarse", bad)
		}
	}
}

func putUpload(h http.Handler, target, contentType, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("PUT", target, strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLocalUpload(t *testing.T) {
	a := appNoDB(t)
	ls := a.Store.(*localStorage)
	h := a.Routes()
	ctx := context.Background()

	uploadURL, headers, err := a.Store.PresignUpload(ctx, "u/2026/11/abc.png", "image/png", 4)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(uploadURL, "/api/uploads/local?token=") || headers["Content-Type"] != "image/png" {
		t.Fatalf("ticket inesperado: %s %v", uploadURL, headers)
	}
	if rec := putUpload(h, uploadURL, "image/jpeg", "abcd"); rec.Code != http.StatusBadRequest {
		t.Fatalf("tipo distinto: %d", rec.Code)
	}
	if rec := putUpload(h, uploadURL, "image/png", "abcde"); rec.Code != http.StatusBadRequest {
		t.Fatalf("tamaño distinto: %d", rec.Code)
	}
	if rec := putUpload(h, uploadURL+"x", "image/png", "abcd"); rec.Code != http.StatusForbidden {
		t.Fatalf("token alterado: %d", rec.Code)
	}
	if rec := putUpload(h, uploadURL, "image/png", "abcd"); rec.Code != http.StatusOK {
		t.Fatalf("subida válida: %d %s", rec.Code, rec.Body)
	}

	info, err := a.Store.Stat(ctx, "u/2026/11/abc.png")
	if err != nil || info.Size != 4 || info.ContentType != "image/png" {
		t.Fatalf("Stat: %+v %v", info, err)
	}
	if _, err := a.Store.Stat(ctx, "u/2026/11/otro.png"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Stat de algo inexistente: %v", err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/media/u/2026/11/abc.png", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "abcd" {
		t.Fatalf("GET /media: %d %q", rec.Code, rec.Body)
	}

	expired := signToken(ls.key, uploadClaims{Key: "u/x.png", ContentType: "image/png", Size: 4, Exp: time.Now().Add(-time.Second).Unix()})
	if rec := putUpload(h, "/api/uploads/local?token="+url.QueryEscape(expired), "image/png", "abcd"); rec.Code != http.StatusForbidden {
		t.Fatalf("token vencido: %d", rec.Code)
	}
}

func TestLocalStorageTraversal(t *testing.T) {
	a := appNoDB(t)
	ls := a.Store.(*localStorage)
	h := a.Routes()
	ctx := context.Background()
	outside := t.TempDir()

	if _, _, err := ls.PresignUpload(ctx, "../evil.png", "image/png", 4); err == nil {
		t.Fatal("PresignUpload aceptó una clave con ..")
	}
	// Aunque alguien tuviera un token firmado con una clave mala, no sale de MEDIA_DIR.
	for _, key := range []string{"../evil.png", "/etc/evil.png", "a/../../evil.png"} {
		tok := signToken(ls.key, uploadClaims{Key: key, ContentType: "image/png", Size: 4, Exp: time.Now().Add(time.Minute).Unix()})
		if rec := putUpload(h, "/api/uploads/local?token="+url.QueryEscape(tok), "image/png", "abcd"); rec.Code != http.StatusBadRequest {
			t.Errorf("clave %q: %d", key, rec.Code)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(a.Cfg.MediaDir), "evil.png")); err == nil {
		t.Fatal("se escribió fuera de MEDIA_DIR")
	}

	// Un enlace simbólico hacia afuera tampoco sirve para leer.
	if err := os.WriteFile(filepath.Join(outside, "secreto.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(a.Cfg.MediaDir, "fuga")); err != nil {
		t.Fatal(err)
	}
	if _, err := ls.Stat(ctx, "fuga/secreto.png"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Stat siguió un enlace hacia afuera: %v", err)
	}
	for _, p := range []string{"/media/fuga/secreto.png", "/media/..%2fapp.go"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code == http.StatusOK {
			t.Errorf("GET %s devolvió 200", p)
		}
	}
}

func TestLocalList(t *testing.T) {
	cfg := isolatedConfig(t)
	cfg.MediaSeedDir = t.TempDir()
	write := func(dir, name string) {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(cfg.MediaSeedDir, "legacy/avatars/DefaultH1.png")
	write(cfg.MediaSeedDir, "legacy/badges/internet.png")
	write(cfg.MediaDir, "legacy/avatars/0001-1.png")
	write(cfg.MediaDir, "legacy/avatars/.upload-tmp")
	write(cfg.MediaDir, "u/2026/11/a.png")

	ls, err := newLocalStorage(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ls.List(context.Background(), "legacy/avatars/")
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, o := range got {
		keys = append(keys, o.Key)
	}
	if strings.Join(keys, ",") != "legacy/avatars/0001-1.png,legacy/avatars/DefaultH1.png" {
		t.Fatalf("List = %v", keys)
	}
	if got, _ := ls.List(context.Background(), "nada/"); len(got) != 0 {
		t.Fatalf("List de un prefijo inexistente: %v", got)
	}
	if info, err := ls.Stat(context.Background(), "legacy/badges/internet.png"); err != nil || info.Size != 1 {
		t.Fatalf("Stat desde MEDIA_SEED_DIR: %+v %v", info, err)
	}
}

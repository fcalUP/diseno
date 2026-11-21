package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Las claves no llevan prefijo: "u/2026/11/ab12.png" se publica como "/media/u/2026/11/ab12.png".
type ObjectInfo struct {
	Key         string
	Size        int64
	ContentType string
}

type Storage interface {
	PresignUpload(ctx context.Context, key, contentType string, size int64) (url string, headers map[string]string, err error)
	Stat(ctx context.Context, key string) (ObjectInfo, error) // ErrNotFound si no existe
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
}

var ErrNotFound = errors.New("not found")

const uploadTTL = 15 * time.Minute

func mediaURL(key string) string { return "/media/" + key }

// mediaKey es el inverso de mediaURL; false si no es una url /media/ válida.
func mediaKey(u string) (string, bool) {
	k, ok := strings.CutPrefix(u, "/media/")
	return k, ok && validKey(k)
}

// validKey rechaza claves vacías, absolutas o con "." / ".." (path traversal).
func validKey(k string) bool {
	return k != "" && k != "." && fs.ValidPath(k) && !strings.Contains(k, `\`)
}

func keyContentType(key string) string {
	if t := mime.TypeByExtension(path.Ext(key)); t != "" {
		return t
	}
	return "application/octet-stream"
}

// ---------- S3 ----------

type s3Storage struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
	prefix  string // S3_PREFIX, p. ej. "media/"
}

func newS3Storage(ctx context.Context, cfg Config) (*s3Storage, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		return nil, fmt.Errorf("configuración de AWS: %w", err)
	}
	client := s3.NewFromConfig(awsCfg)
	return &s3Storage{client: client, presign: s3.NewPresignClient(client), bucket: cfg.S3Bucket, prefix: cfg.S3Prefix}, nil
}

// PresignUpload firma un PUT con tipo y tamaño exactos, válido 15 minutos.
// Devuelve las cabeceras firmadas menos Host y Content-Length, que pone el navegador.
func (s *s3Storage) PresignUpload(ctx context.Context, key, contentType string, size int64) (string, map[string]string, error) {
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(s.prefix + key),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(size),
	}, s3.WithPresignExpires(uploadTTL))
	if err != nil {
		return "", nil, err
	}
	headers := map[string]string{}
	for k, v := range req.SignedHeader {
		if strings.EqualFold(k, "Host") || strings.EqualFold(k, "Content-Length") {
			continue
		}
		headers[http.CanonicalHeaderKey(k)] = strings.Join(v, ",")
	}
	return req.URL, headers, nil
}

func (s *s3Storage) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(s.prefix + key)})
	if err != nil {
		var nf *types.NotFound
		var status interface{ HTTPStatusCode() int }
		if errors.As(err, &nf) || (errors.As(err, &status) && status.HTTPStatusCode() == http.StatusNotFound) {
			return ObjectInfo{}, ErrNotFound
		}
		return ObjectInfo{}, err
	}
	return ObjectInfo{Key: key, Size: aws.ToInt64(out.ContentLength), ContentType: aws.ToString(out.ContentType)}, nil
}

// List devuelve las claves sin el prefijo de S3.
func (s *s3Storage) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var out []ObjectInfo
	p := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(s.prefix + prefix),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, o := range page.Contents {
			k := strings.TrimPrefix(aws.ToString(o.Key), s.prefix)
			if k == "" || strings.HasSuffix(k, "/") { // "carpetas" vacías de la consola
				continue
			}
			out = append(out, ObjectInfo{Key: k, Size: aws.ToInt64(o.Size), ContentType: keyContentType(k)})
		}
	}
	return out, nil
}

// ---------- Local (desarrollo) ----------

// localStorage guarda en MEDIA_DIR y lee también de MEDIA_SEED_DIR. os.Root
// impide salir de esas carpetas, incluso con enlaces simbólicos.
type localStorage struct {
	media *os.Root
	seed  *os.Root // nil si no hay MEDIA_SEED_DIR
	key   []byte   // firma de los tokens de subida
}

type uploadClaims struct {
	Key         string `json:"k"`
	ContentType string `json:"t"`
	Size        int64  `json:"s"`
	Exp         int64  `json:"e"`
}

func newLocalStorage(cfg Config) (*localStorage, error) {
	if err := os.MkdirAll(cfg.MediaDir, 0o755); err != nil {
		return nil, fmt.Errorf("MEDIA_DIR: %w", err)
	}
	media, err := os.OpenRoot(cfg.MediaDir)
	if err != nil {
		return nil, fmt.Errorf("MEDIA_DIR: %w", err)
	}
	ls := &localStorage{media: media, key: deriveKey("diseno-upload:", cfg.SessionSecret)}
	if cfg.MediaSeedDir != "" {
		if ls.seed, err = os.OpenRoot(cfg.MediaSeedDir); err != nil {
			return nil, fmt.Errorf("MEDIA_SEED_DIR: %w", err)
		}
	}
	return ls, nil
}

func (s *localStorage) roots() []*os.Root {
	if s.seed == nil {
		return []*os.Root{s.media}
	}
	return []*os.Root{s.media, s.seed}
}

// PresignUpload devuelve una url local con un token firmado (clave, tipo, tamaño, vence en 15 min).
func (s *localStorage) PresignUpload(ctx context.Context, key, contentType string, size int64) (string, map[string]string, error) {
	if !validKey(key) {
		return "", nil, fmt.Errorf("clave inválida: %q", key)
	}
	tok := signToken(s.key, uploadClaims{Key: key, ContentType: contentType, Size: size, Exp: time.Now().Add(uploadTTL).Unix()})
	return localUploadURL + "?token=" + url.QueryEscape(tok), map[string]string{"Content-Type": contentType}, nil
}

func (s *localStorage) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	_, info, err := s.find(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	return ObjectInfo{Key: key, Size: info.Size(), ContentType: keyContentType(key)}, nil
}

// find busca un archivo normal primero en MEDIA_DIR y luego en MEDIA_SEED_DIR.
func (s *localStorage) find(key string) (*os.Root, fs.FileInfo, error) {
	if !validKey(key) {
		return nil, nil, ErrNotFound
	}
	for _, root := range s.roots() {
		if info, err := root.Stat(key); err == nil && info.Mode().IsRegular() {
			return root, info, nil
		}
	}
	return nil, nil, ErrNotFound
}

func (s *localStorage) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	dir := "."
	if i := strings.LastIndex(prefix, "/"); i > 0 {
		dir = prefix[:i]
	}
	seen := map[string]bool{}
	var out []ObjectInfo
	for _, root := range s.roots() {
		err := fs.WalkDir(root.FS(), dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if d.IsDir() || strings.HasPrefix(d.Name(), ".") || !strings.HasPrefix(p, prefix) || seen[p] {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			seen[p] = true
			out = append(out, ObjectInfo{Key: p, Size: info.Size(), ContentType: keyContentType(p)})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	slices.SortFunc(out, func(a, b ObjectInfo) int { return cmp.Compare(a.Key, b.Key) })
	return out, nil
}

var errSizeMismatch = errors.New("el tamaño no coincide")

// write guarda el cuerpo en un temporal y lo renombra, así nunca queda un archivo a medias.
func (s *localStorage) write(key string, body io.Reader, size int64) error {
	dir := path.Dir(key)
	if err := s.media.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path.Join(dir, ".upload-"+randomToken(8))
	f, err := s.media.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(body, size+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n != size {
		err = errSizeMismatch
	}
	if err == nil {
		err = s.media.Rename(tmp, key)
	}
	if err != nil {
		_ = s.media.Remove(tmp)
	}
	return err
}

// handleLocalUpload recibe el PUT de PresignUpload en modo local. El token es la autorización.
func (a *App) handleLocalUpload(w http.ResponseWriter, r *http.Request) {
	ls, ok := a.Store.(*localStorage)
	if !ok {
		writeError(w, http.StatusNotFound, "Ruta no encontrada.")
		return
	}
	var c uploadClaims
	if !verifyToken(ls.key, r.URL.Query().Get("token"), &c) || time.Now().Unix() >= c.Exp {
		writeError(w, http.StatusForbidden, "El enlace de subida no es válido o ya venció.")
		return
	}
	if !validKey(c.Key) {
		writeError(w, http.StatusBadRequest, "Ruta de archivo inválida.")
		return
	}
	if r.Header.Get("Content-Type") != c.ContentType {
		writeError(w, http.StatusBadRequest, "El tipo de archivo no coincide con el autorizado.")
		return
	}
	if r.ContentLength >= 0 && r.ContentLength != c.Size {
		writeError(w, http.StatusBadRequest, "El tamaño del archivo no coincide con el autorizado.")
		return
	}
	err := ls.write(c.Key, r.Body, c.Size)
	var tooBig *http.MaxBytesError
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case errors.Is(err, errSizeMismatch):
		writeError(w, http.StatusBadRequest, "El tamaño del archivo no coincide con el autorizado.")
	case errors.As(err, &tooBig):
		writeError(w, http.StatusRequestEntityTooLarge, "El archivo es demasiado grande.")
	default:
		a.Log.Error("subida local", "key", c.Key, "err", err)
		writeError(w, http.StatusInternalServerError, "No se pudo guardar el archivo.")
	}
}

package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"
)

// notBuilt is served when the binary was compiled without the web app (plain `go build`).
const notBuilt = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Gokku</title></head>
<body><p>The web app is not built into this binary. Run <code>task build</code>.</p></body></html>
`

// spaHandler serves the built SPA: real files as is, any other GET path as index.html so
// client-side routes work on reload.
type spaHandler struct {
	files fs.FS
	index []byte
	csp   string
}

func newSPAHandler(files fs.FS) (*spaHandler, error) {
	index, err := fs.ReadFile(files, "index.html")
	if errors.Is(err, fs.ErrNotExist) {
		index = []byte(notBuilt)
	} else if err != nil {
		return nil, err
	}
	return &spaHandler{files: files, index: index, csp: contentSecurityPolicy(index)}, nil
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name != "" && name != "index.html" {
		if info, err := fs.Stat(h.files, name); err == nil && !info.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				// Vite puts a content hash in every file name under assets/.
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			http.ServeFileFS(w, r, h.files, name) //nolint:gosec // io/fs paths cannot leave the embedded root

			return
		}
		// A missing file with an extension is a real 404, not a client route.
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(h.index))
}

var inlineScript = regexp.MustCompile(`(?is)<script([^>]*)>(.*?)</script>`)

// contentSecurityPolicy allows only same-origin resources (SEC-6). React Router's SPA build
// and the theme script (DESIGN.md, 5.3) put inline scripts in index.html, so each one is
// allowed by its sha256 hash instead of 'unsafe-inline'.
func contentSecurityPolicy(index []byte) string {
	scriptSrc := []string{"'self'"}
	for _, m := range inlineScript.FindAllSubmatch(index, -1) {
		if bytes.Contains(bytes.ToLower(m[1]), []byte("src=")) || len(m[2]) == 0 {
			continue
		}
		sum := sha256.Sum256(m[2])
		scriptSrc = append(scriptSrc, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	return strings.Join([]string{
		"default-src 'self'",
		"script-src " + strings.Join(scriptSrc, " "),
		"style-src 'self'",
		"img-src 'self' data:",
		"font-src 'self'",
		"connect-src 'self'",
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"frame-ancestors 'none'",
	}, "; ")
}

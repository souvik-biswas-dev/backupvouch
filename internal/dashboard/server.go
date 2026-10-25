/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dashboard

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
)

// webFiles holds the page, its scripts, styles and fonts. Nothing is loaded
// from a CDN, because clusters may be offline (D22).
//
//go:embed web
var webFiles embed.FS

var log = ctrl.Log.WithName("dashboard")

// contentSecurityPolicy allows only same-origin files. There is no inline
// script or style, so the page also works under this strict policy.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; font-src 'self'; " +
	"img-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

func init() {
	// Distroless images have no /etc/mime.types, and Go's built-in table lacks these two.
	_ = mime.AddExtensionType(".woff2", "font/woff2")
	_ = mime.AddExtensionType(".ico", "image/x-icon")
}

// Server is a read-only HTTP server. It implements manager.Runnable and
// manager.LeaderElectionRunnable, so every replica serves, not only the leader.
type Server struct {
	// Reader reads BackupPolicy objects; pass the manager's (cached) client.
	Reader client.Reader
	// Addr is the listen address, for example 127.0.0.1:9090.
	Addr string
	// Now is the clock; nil means time.Now. Tests set it.
	Now func() time.Time
}

// NeedLeaderElection reports false: reading status does not need the lease.
func (s *Server) NeedLeaderElection() bool { return false }

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Handler returns the full handler: the JSON endpoint and the embedded files,
// GET and HEAD only, with security headers.
func (s *Server) Handler() (http.Handler, error) {
	sub, err := fs.Sub(webFiles, "web")
	if err != nil {
		return nil, fmt.Errorf("opening embedded dashboard files: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/policies", s.handlePolicies)
	mux.Handle("/", http.FileServerFS(sub))
	return readOnly(mux), nil
}

// Start serves until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	h, err := s.Handler()
	if err != nil {
		return err
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("listening on %q for the dashboard: %w", s.Addr, err)
	}
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	log.Info("Dashboard listening", "address", ln.Addr().String())

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down the dashboard: %w", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serving the dashboard: %w", err)
	}
}

func (s *Server) handlePolicies(w http.ResponseWriter, r *http.Request) {
	var list backupv1alpha1.BackupPolicyList
	if err := s.Reader.List(r.Context(), &list); err != nil {
		log.Error(err, "Failed to list BackupPolicies")
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "The operator could not read the BackupPolicy objects. Check the operator logs.",
		})
		return
	}
	writeJSON(w, http.StatusOK, BuildResponse(list.Items, s.now()))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		log.Error(err, "Failed to encode dashboard response")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// readOnly rejects everything except GET and HEAD, hides directory listings
// and adds the security headers to every response.
func readOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			h.Set("Allow", "GET, HEAD")
			http.Error(w, "read-only dashboard", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/" && strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

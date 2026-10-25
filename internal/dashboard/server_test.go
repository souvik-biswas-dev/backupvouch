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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	backupv1alpha1 "github.com/souvik-biswas-dev/backupvouch/api/v1alpha1"
)

var fixedNow = time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)

func newTestServer(t *testing.T, objs ...client.Object) http.Handler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := backupv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	// VERIFY: the fake client keeps the status of objects passed to WithObjects.
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	h, err := (&Server{Reader: reader, Addr: "127.0.0.1:0", Now: func() time.Time { return fixedNow }}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func do(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestPoliciesEndpoint(t *testing.T) {
	failed := newPolicy("orders", func(bp *backupv1alpha1.BackupPolicy) {
		bp.Status.LastBackup = &backupv1alpha1.BackupStatus{Result: backupv1alpha1.BackupSucceeded, ObjectKey: "k"}
		bp.Status.LastVerification = &backupv1alpha1.VerificationStatus{
			Result: backupv1alpha1.VerificationFailed, ObjectKey: "k", FailedChecks: []string{"row_count_orders"},
		}
	})
	ok := newPolicy("billing", func(bp *backupv1alpha1.BackupPolicy) {
		bp.Status.LastBackup = &backupv1alpha1.BackupStatus{Result: backupv1alpha1.BackupSucceeded, ObjectKey: "k"}
		bp.Status.LastVerification = &backupv1alpha1.VerificationStatus{Result: backupv1alpha1.VerificationPassed, ObjectKey: "k"}
	})
	h := newTestServer(t, &failed, &ok)

	rec := do(h, http.MethodGet, "/api/v1/policies")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cc)
	}
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp.Summary.Total != 2 || resp.Summary.NeedAttention != 1 || resp.Summary.Vouched != 1 {
		t.Fatalf("summary = %+v", resp.Summary)
	}
	if resp.Policies[0].Name != "orders" || resp.Policies[0].Verdict != VerdictFailed {
		t.Fatalf("the failing policy should come first, got %+v", resp.Policies[0])
	}
	if got := resp.Policies[0].LastVerification.FailedChecks; len(got) != 1 || got[0] != "row_count_orders" {
		t.Fatalf("failed checks = %v", got)
	}
	if !resp.GeneratedAt.Equal(fixedNow) {
		t.Fatalf("generatedAt = %v", resp.GeneratedAt)
	}
}

func TestEmptyCluster(t *testing.T) {
	rec := do(newTestServer(t), http.MethodGet, "/api/v1/policies")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"policies":[]`) {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
}

func TestReadOnly(t *testing.T) {
	h := newTestServer(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		for _, path := range []string{"/api/v1/policies", "/", "/app.css"} {
			rec := do(h, method, path)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s = %d, want 405", method, path, rec.Code)
			}
			if allow := rec.Header().Get("Allow"); allow != "GET, HEAD" {
				t.Errorf("%s %s Allow = %q", method, path, allow)
			}
		}
	}
}

func TestStaticFilesAndHeaders(t *testing.T) {
	h := newTestServer(t)

	rec := do(h, http.MethodGet, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "/js/app.js") {
		t.Fatalf("index.html does not load /js/app.js")
	}
	for header, want := range map[string]string{
		"Content-Security-Policy": "default-src 'none'",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "no-referrer",
	} {
		if got := rec.Header().Get(header); !strings.Contains(got, want) {
			t.Errorf("%s = %q, want it to contain %q", header, got, want)
		}
	}

	for path, wantType := range map[string]string{
		"/app.css":     "text/css",
		"/js/app.js":   "text/javascript",
		"/favicon.svg": "image/svg+xml",
		"/favicon.ico": "image/x-icon",
	} {
		rec := do(h, http.MethodGet, path)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d", path, rec.Code)
			continue
		}
		if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, wantType) {
			t.Errorf("GET %s Content-Type = %q, want prefix %q", path, got, wantType)
		}
	}

	if rec := do(h, http.MethodHead, "/"); rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Errorf("HEAD / = %d with %d body bytes", rec.Code, rec.Body.Len())
	}
}

func TestNoDirectoryListingAndUnknownPaths(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{"/js/", "/fonts/", "/nope", "/api/v1/nope/"} {
		if rec := do(h, http.MethodGet, path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
}

type failingReader struct{}

func (failingReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return errors.New("boom: secret-looking detail")
}

func (failingReader) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return errors.New("boom: secret-looking detail")
}

func TestListErrorIsReportedWithoutDetails(t *testing.T) {
	h, err := (&Server{Reader: failingReader{}}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	rec := do(h, http.MethodGet, "/api/v1/policies")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "boom") {
		t.Fatalf("error detail leaked to the client: %s", rec.Body)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" {
		t.Fatalf("want a JSON body with an error message, got %s (%v)", rec.Body, err)
	}
}

func TestEveryReplicaServes(t *testing.T) {
	if (&Server{}).NeedLeaderElection() {
		t.Fatal("the dashboard must not wait for leader election")
	}
}

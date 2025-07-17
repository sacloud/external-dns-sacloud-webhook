// Copyright 2025- The sacloud/external-dns-sacloud-webhook authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sacloud/external-dns-sacloud-webhook/internal/config"
	"github.com/sacloud/external-dns-sacloud-webhook/internal/provider"
)

func TestRootEndpoint(t *testing.T) {
	cfg := config.Config{ZoneName: "test.com"}
	client := &provider.Client{ZoneName: cfg.ZoneName}
	mux := NewMux(client, cfg)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET / returned %d; want 200", rr.Code)
	}
	contentType := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/external.dns.webhook+json") {
		t.Errorf("unexpected Content-Type: %q", contentType)
	}
	body, _ := io.ReadAll(rr.Body)
	want := `{"domainFilter":["test.com"],"recordTypes":["A","CNAME","TXT"]}`
	if strings.TrimSpace(string(body)) != want {
		t.Errorf("body = %q; want %q", string(body), want)
	}
}

func TestHealthzEndpoint(t *testing.T) {
	cfg := config.Config{ZoneName: "whatever"}
	client := &provider.Client{ZoneName: cfg.ZoneName}
	mux := NewMux(client, cfg)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /healthz returned %d; want 200", rr.Code)
	}
	body, _ := io.ReadAll(rr.Body)
	if strings.TrimSpace(string(body)) != `{"status":"ok"}` {
		t.Errorf("body = %q; want {\"status\":\"ok\"}", string(body))
	}
}

func TestNotAllowedMethod(t *testing.T) {
	// Sending a PUT request to the /records endpoint should return 405
	cfg := config.Config{ZoneName: "z"}
	client := &provider.Client{ZoneName: cfg.ZoneName}
	mux := NewMux(client, cfg)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/records", nil)
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /records returned %d; want 405", rr.Code)
	}
}

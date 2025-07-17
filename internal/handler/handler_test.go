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

package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sacloud/external-dns-sacloud-webhook/internal/provider"
	"sigs.k8s.io/external-dns/endpoint"
)

// fakeProvider is a mock implementation of the handler.Provider interface.
// The behavior of ListRecords, ApplyChanges, and GetZoneName can be controlled via struct fields.
type fakeProvider struct {
	// For RecordsHandler tests
	records []provider.Record
	listErr error

	// For ApplyHandler tests
	createIn []provider.Record
	deleteIn []provider.Record
	applyErr error
}

func (f *fakeProvider) ListRecords() ([]provider.Record, error) {
	return f.records, f.listErr
}

func (f *fakeProvider) ApplyChanges(create, del []provider.Record) error {
	f.createIn = create
	f.deleteIn = del
	return f.applyErr
}

func (f *fakeProvider) GetZoneName() string {
	// Used by RecordsHandler to construct the FQDN suffix
	return "example.com"
}

// --- Tests for RecordsHandler ---

func TestRecordsHandler_Success(t *testing.T) {
	fake := &fakeProvider{
		records: []provider.Record{
			{Type: "A", Name: "foo", Targets: []string{"1.2.3.4"}},
			{Type: "CNAME", Name: "www", Targets: []string{"example.com."}},
		},
	}
	handler := RecordsHandler(fake)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/records", nil)
	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/external.dns.webhook+json;version=1" {
		t.Errorf("unexpected Content-Type: %q", ct)
	}

	var eps []endpoint.Endpoint
	if err := json.Unmarshal(rr.Body.Bytes(), &eps); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if len(eps) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(eps))
	}
}

func TestRecordsHandler_Error(t *testing.T) {
	fake := &fakeProvider{listErr: errors.New("fail")}
	handler := RecordsHandler(fake)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/records", nil)
	handler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 Internal Server Error, got %d", rr.Code)
	}
}

// --- Tests for AdjustHandler ---

func TestAdjustHandler_PassThrough(t *testing.T) {
	fake := &fakeProvider{}
	handler := AdjustHandler(fake)

	input := []endpoint.Endpoint{
		{DNSName: "a.example.com", Targets: []string{"1.1.1.1"}, RecordType: "A"},
	}
	body, _ := json.Marshal(input)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/adjustendpoints", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/external.dns.webhook+json;version=1")
	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	var out []endpoint.Endpoint
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if len(out) != len(input) {
		t.Errorf("expected %d items, got %d", len(input), len(out))
	}
}

func TestAdjustHandler_BadContentType(t *testing.T) {
	fake := &fakeProvider{}
	handler := AdjustHandler(fake)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/adjustendpoints", nil)
	req.Header.Set("Content-Type", "text/plain")
	handler(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415 Unsupported Media Type, got %d", rr.Code)
	}
}

// --- Tests for ApplyHandler ---

func TestApplyHandler_Success(t *testing.T) {
	fake := &fakeProvider{}
	handler := ApplyHandler(fake)

	cr := ChangeRequest{
		Create: []*endpoint.Endpoint{
			{DNSName: "x.example.com", Targets: []string{"2.2.2.2"}, RecordType: "A"},
		},
		Delete: []*endpoint.Endpoint{
			{DNSName: "y.example.com", Targets: []string{"3.3.3.3"}, RecordType: "A"},
		},
	}
	body, _ := json.Marshal(cr)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/records", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/external.dns.webhook+json;version=1")
	handler(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d", rr.Code)
	}
	// Verify that fakeProvider received the correct create/delete input
	if len(fake.createIn) != 1 || len(fake.deleteIn) != 1 {
		t.Errorf("unexpected create/delete slices: %+v / %+v", fake.createIn, fake.deleteIn)
	}
}

func TestApplyHandler_BadJSON(t *testing.T) {
	fake := &fakeProvider{}
	handler := ApplyHandler(fake)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/records", bytes.NewReader([]byte(`{bad`)))
	req.Header.Set("Content-Type", "application/external.dns.webhook+json;version=1")
	handler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request on malformed JSON, got %d", rr.Code)
	}
}

func TestApplyHandler_ProviderError(t *testing.T) {
	fake := &fakeProvider{applyErr: errors.New("oops")}
	handler := ApplyHandler(fake)

	cr := ChangeRequest{Create: nil, Delete: nil}
	body, _ := json.Marshal(cr)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/records", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/external.dns.webhook+json;version=1")
	handler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 Internal Server Error on provider error, got %d", rr.Code)
	}
}

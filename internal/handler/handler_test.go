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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/sacloud/external-dns-sacloud-webhook/internal/provider"
	"sigs.k8s.io/external-dns/endpoint"
)

type fakeProvider struct {
	// For RecordsHandler tests
	records []provider.Record
	listErr error

	// For ApplyHandler tests
	createIn []provider.Record
	deleteIn []provider.Record
	applyErr error
}

func (f *fakeProvider) ListRecords(ctx context.Context) ([]provider.Record, error) {
	return f.records, f.listErr
}

func (f *fakeProvider) ApplyChanges(ctx context.Context, create, del []provider.Record) error {
	f.createIn = create
	f.deleteIn = del
	return f.applyErr
}

func (f *fakeProvider) GetZoneName() string {
	return "example.com"
}

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

func TestApplyHandler_Success_CreateDeleteOnly(t *testing.T) {
	fake := &fakeProvider{}
	handler := ApplyHandler(fake)

	cr := ChangeRequest{
		Create: []*endpoint.Endpoint{
			{DNSName: "x.example.com", Targets: []string{"2.2.2.2"}, RecordType: "A", RecordTTL: 300},
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

	// Verify provider inputs
	if len(fake.createIn) != 1 || len(fake.deleteIn) != 1 {
		t.Fatalf("unexpected create/delete slices: %+v / %+v", fake.createIn, fake.deleteIn)
	}
	// Name should be trimmed to relative (without ".example.com")
	if fake.createIn[0].Name != "x" || fake.deleteIn[0].Name != "y" {
		t.Errorf("unexpected names: create=%q delete=%q", fake.createIn[0].Name, fake.deleteIn[0].Name)
	}
	// TTL fallback/override check
	if fake.createIn[0].TTL != 300 {
		t.Errorf("expected TTL=300, got %d", fake.createIn[0].TTL)
	}
}

func TestApplyHandler_Success_WithUpdates_MappedToDeleteCreate(t *testing.T) {
	fake := &fakeProvider{}
	handler := ApplyHandler(fake)

	cr := ChangeRequest{
		// No direct create/delete
		Create: nil,
		Delete: nil,
		// Simulate TTL update for the same CNAME with alias=true
		UpdateOld: []*endpoint.Endpoint{
			{
				DNSName:      "cname.example.com",
				RecordType:   "CNAME",
				RecordTTL:    120,
				Targets:      endpoint.Targets{"target.example.com."},
				ProviderSpecific: endpoint.ProviderSpecific{
					{Name: "alias", Value: "true"},
				},
			},
		},
		UpdateNew: []*endpoint.Endpoint{
			{
				DNSName:      "cname.example.com",
				RecordType:   "CNAME",
				RecordTTL:    3600,
				Targets:      endpoint.Targets{"target.example.com"}, // no trailing dot -> should be normalized to end with dot
				ProviderSpecific: endpoint.ProviderSpecific{
					{Name: "alias", Value: "true"},
				},
			},
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

	// Expect: update transformed into delete+create
	if len(fake.deleteIn) != 1 || len(fake.createIn) != 1 {
		t.Fatalf("expected 1 delete and 1 create, got delete=%d create=%d", len(fake.deleteIn), len(fake.createIn))
	}

	del := fake.deleteIn[0]
	crt := fake.createIn[0]

	// Both names should be relative (zone suffix trimmed)
	if del.Name != "cname" || crt.Name != "cname" {
		t.Errorf("unexpected names after trim: delete=%q create=%q", del.Name, crt.Name)
	}

	// Types: alias=true on CNAME is mapped to ALIAS
	if del.Type != "ALIAS" || crt.Type != "ALIAS" {
		t.Errorf("expected ALIAS types, got delete=%q create=%q", del.Type, crt.Type)
	}

	// Targets should end with trailing dot for CNAME/ALIAS
	if len(crt.Targets) != 1 || crt.Targets[0] != "target.example.com." {
		t.Errorf("unexpected create target normalization: %+v", crt.Targets)
	}

	// TTL updated
	if del.TTL != 120 || crt.TTL != 3600 {
		t.Errorf("unexpected TTLs: delete=%d create=%d", del.TTL, crt.TTL)
	}
}

func TestApplyHandler_BadContentType(t *testing.T) {
	fake := &fakeProvider{}
	handler := ApplyHandler(fake)

	body, _ := json.Marshal(ChangeRequest{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/records", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json") // wrong
	handler(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415 Unsupported Media Type, got %d", rr.Code)
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

func TestRecordsHandler_ContextTimeout(t *testing.T) {
	fake := &fakeProvider{
		listErr: context.Canceled,
	}
	handler := RecordsHandler(fake)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/records", nil)
	handler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 Internal Server Error on context canceled, got %d", rr.Code)
	}
}

func TestApplyHandler_ContextTimeout(t *testing.T) {
	fake := &fakeProvider{
		applyErr: context.Canceled,
	}
	handler := ApplyHandler(fake)

	cr := ChangeRequest{Create: nil, Delete: nil}
	body, _ := json.Marshal(cr)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/records", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/external.dns.webhook+json;version=1")
	handler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 Internal Server Error on context canceled, got %d", rr.Code)
	}
}

// Optional sanity test: ensure convertEndpoints keeps TXT ownership formatting.
// (This is a white-box-ish test; if you prefer black-box, you can rely on the ALIAS update test above.)
func Test_convertEndpoints_TXT_and_AliasNormalization(t *testing.T) {
	zoneSuffix := ".example.com"
	txtPrefix := "_external-dns."

	in := []*endpoint.Endpoint{
		{
			DNSName:    "_external-dns.cname-foo.example.com",
			RecordType: "TXT",
			Targets:    endpoint.Targets{`"heritage=external-dns,owner=default"`},
		},
		{
			DNSName:    "bar.example.com",
			RecordType: "CNAME",
			Targets:    endpoint.Targets{"target.example.com"}, // no trailing dot
			ProviderSpecific: endpoint.ProviderSpecific{
				{Name: "alias", Value: "true"},
			},
		},
	}

	got := convertEndpoints(in, zoneSuffix, txtPrefix)

	want := []provider.Record{
		{Type: "TXT", Name: "_external-dns.cname-foo", Targets: []string{"heritage=external-dns,owner=default"}, TTL: 3600},
		{Type: "ALIAS", Name: "bar", Targets: []string{"target.example.com."}, TTL: 3600},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("convertEndpoints mismatch:\n got = %#v\nwant = %#v", got, want)
	}
}

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

package provider

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	iaas "github.com/sacloud/iaas-api-go"
	"github.com/sacloud/iaas-api-go/types"
	"github.com/sacloud/iaas-service-go/dns"
)

type fakeDNSService struct {
	findResp      []*iaas.DNS
	findErr       error
	readResp      *iaas.DNS
	readErr       error
	updateResp    *iaas.DNS
	updateErr     error
	lastUpdateReq *dns.UpdateRequest
}

func (f *fakeDNSService) FindWithContext(ctx context.Context, req *dns.FindRequest) ([]*iaas.DNS, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return f.findResp, f.findErr
}

func (f *fakeDNSService) ReadWithContext(ctx context.Context, req *dns.ReadRequest) (*iaas.DNS, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return f.readResp, f.readErr
}

func (f *fakeDNSService) UpdateWithContext(ctx context.Context, req *dns.UpdateRequest) (*iaas.DNS, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	f.lastUpdateReq = req
	return f.updateResp, f.updateErr
}
func TestListRecords(t *testing.T) {
	fake := &fakeDNSService{
		readResp: &iaas.DNS{
			ID:   123,
			Name: "example.com",
			Records: []*iaas.DNSRecord{
				{
					Name:  "www",
					Type:  types.EDNSRecordType("A"),
					RData: "1.2.3.4",
				},
				{
					Name:  "@",
					Type:  types.EDNSRecordType("TXT"),
					RData: "owner=foo",
				},
			},
		},
	}

	client := &Client{
		Context:        context.Background(),
		Service:        fake,
		ZoneName:       "example.com",
		ZoneID:         123,
		RequestTimeout: 5 * time.Second,
	}

	records, err := client.ListRecords(context.Background())
	if err != nil {
		t.Fatalf("ListRecords() unexpected error: %v", err)
	}

	want := []Record{
		{Name: "www", Targets: []string{"1.2.3.4"}, Type: "A"},
		{Name: "@", Targets: []string{"owner=foo"}, Type: "TXT"},
	}
	if !reflect.DeepEqual(records, want) {
		t.Errorf("ListRecords() = %#v; want %#v", records, want)
	}
}

func TestApplyChanges(t *testing.T) {
	fake := &fakeDNSService{
		readResp: &iaas.DNS{
			ID:   999,
			Name: "mixed.com",
			Records: []*iaas.DNSRecord{
				{Name: "keep", Type: types.EDNSRecordType("A"), RData: "1.1.1.1"},
				{Name: "delA", Type: types.EDNSRecordType("A"), RData: "2.2.2.2"},
				{Name: "delTXT", Type: types.EDNSRecordType("TXT"), RData: "foo=bar"},
				{Name: "delC", Type: types.EDNSRecordType("CNAME"), RData: "c.target.com."},
				{Name: "delAlias", Type: types.EDNSRecordType("ALIAS"), RData: "a.target.com."},
			},
		},
		updateResp: &iaas.DNS{},
	}

	client := &Client{
		Context:        context.Background(),
		Service:        fake,
		ZoneName:       "mixed.com",
		ZoneID:         999,
		RequestTimeout: 5 * time.Second,
	}

	toCreate := []Record{
		{Name: "newA", Targets: []string{"3.3.3.3"}, Type: "A"},
		{Name: "newTXT", Targets: []string{"hello=world"}, Type: "TXT"},
		{Name: "newCNAME", Targets: []string{"d.target.com."}, Type: "CNAME"},
		{Name: "newAlias", Targets: []string{"b.target.com."}, Type: "ALIAS"},
	}
	toDelete := []Record{
		{Name: "delA", Targets: []string{"2.2.2.2"}, Type: "A"},
		{Name: "delTXT", Targets: []string{"foo=bar"}, Type: "TXT"},
		{Name: "delC", Targets: []string{"c.target.com."}, Type: "CNAME"},
		{Name: "delAlias", Targets: []string{"a.target.com."}, Type: "ALIAS"},
	}

	if err := client.ApplyChanges(context.Background(), toCreate, toDelete); err != nil {
		t.Fatalf("ApplyChanges() unexpected error: %v", err)
	}

	req := fake.lastUpdateReq
	if req == nil {
		t.Fatal("UpdateRequest was not called")
	}

	// Only the "keep" record and the new records to be created should remain (total 5 records)
	expectedCount := 1 + len(toCreate)
	if len(req.Records) != expectedCount {
		t.Fatalf("UpdateRequest.Records length = %d; want %d", len(req.Records), expectedCount)
	}

	// Expected order: existing "keep" record followed by new records in creation order
	wantOrder := []struct {
		name, rdata, typ string
	}{
		{"keep", "1.1.1.1", "A"},
		{"newA", "3.3.3.3", "A"},
		{"newTXT", "hello=world", "TXT"},
		{"newCNAME", "d.target.com.", "CNAME"},
		{"newAlias", "b.target.com.", "ALIAS"},
	}

	for i, want := range wantOrder {
		rec := req.Records[i]
		if rec.Name != want.name || rec.RData != want.rdata || string(rec.Type) != want.typ {
			t.Errorf("record[%d] = %#v; want Name=%s, RData=%s, Type=%s",
				i, rec, want.name, want.rdata, want.typ)
		}
	}
}

func TestApplyChanges_Error(t *testing.T) {
	fake := &fakeDNSService{
		readResp:  &iaas.DNS{ID: 1, Name: "z", Records: []*iaas.DNSRecord{}},
		updateErr: errors.New("api failure"),
	}

	client := &Client{
		Context:        context.Background(),
		Service:        fake,
		ZoneName:       "z",
		ZoneID:         1,
		RequestTimeout: 1 * time.Second,
	}

	err := client.ApplyChanges(context.Background(), nil, nil)
	if err == nil || err.Error() != "api failure" {
		t.Errorf("ApplyChanges() error = %v; want \"api failure\"", err)
	}
}

func TestFindWithContext_Timeout(t *testing.T) {
    fake := &fakeDNSService{
        findResp: []*iaas.DNS{},
        findErr:  nil,
    }
    client := &Client{
        Context:        context.Background(),
        Service:        fake,
        ZoneName:       "z",
        ZoneID:         1,
        RequestTimeout: 1 * time.Second,
    }
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    _, err := client.Service.FindWithContext(ctx, &dns.FindRequest{})
    if err == nil {
        t.Errorf("FindWithContext() should fail with canceled context")
    }
}

func TestListRecords_ContextTimeout(t *testing.T) {
    fake := &fakeDNSService{
        readResp: &iaas.DNS{ID: 1, Name: "z", Records: []*iaas.DNSRecord{}},
    }
    client := &Client{
        Context:        context.Background(),
        Service:        fake,
        ZoneName:       "z",
        ZoneID:         1,
        RequestTimeout: 1 * time.Second,
    }
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    _, err := client.ListRecords(ctx)
    if err == nil {
        t.Errorf("ListRecords() should fail with canceled context")
    }
}

func TestApplyChanges_ContextTimeout(t *testing.T) {
    fake := &fakeDNSService{
        readResp:  &iaas.DNS{ID: 1, Name: "z", Records: []*iaas.DNSRecord{}},
        updateErr: nil,
    }
    client := &Client{
        Context:        context.Background(),
        Service:        fake,
        ZoneName:       "z",
        ZoneID:         1,
        RequestTimeout: 1 * time.Second,
    }
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    err := client.ApplyChanges(ctx, nil, nil)
    if err == nil {
        t.Errorf("ApplyChanges() should fail with canceled context")
    }
}

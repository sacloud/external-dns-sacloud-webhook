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
	"log"

	iaas "github.com/sacloud/iaas-api-go"
	"github.com/sacloud/iaas-api-go/types"
	"github.com/sacloud/iaas-service-go/dns"
)

// Record represents a DNS record entry.
// Type: record type (A, CNAME, TXT, etc.)
// Name: full record name under the zone
// Targets: record values (IP addresses, CNAME targets, TXT strings)
// TTL: record TTL in seconds
type Record struct {
	Type    string
	Name    string
	Targets []string
	TTL     int
}

// ListRecords fetches all DNS records for the configured zone.
func (c *Client) ListRecords() ([]Record, error) {
	ctx, cancel := context.WithTimeout(c.Context, c.RequestTimeout)
	defer cancel()

	log.Printf("Listing records for zone '%s' (ID: %d)", c.ZoneName, c.ZoneID)
	dnsZone, err := c.Service.ReadWithContext(ctx, &dns.ReadRequest{ID: c.ZoneID})
	if err != nil {
		log.Printf("Error reading DNS zone: %v", err)
		return nil, err
	}

	var records []Record
	for _, rs := range dnsZone.Records {
		rec := Record{
			Type:    string(rs.Type),
			Name:    rs.Name,
			Targets: []string{rs.RData},
			TTL:     rs.TTL,
		}
		log.Printf("Found record: %s %s -> %v (TTL=%d)", rec.Type, rec.Name, rec.Targets, rec.TTL)
		records = append(records, rec)
	}
	return records, nil
}

// ApplyChanges applies create and delete operations to DNS records.
func (c *Client) ApplyChanges(create, del []Record) error {
	ctx, cancel := context.WithTimeout(c.Context, c.RequestTimeout)
	defer cancel()

	log.Printf("Applying changes: create %d, delete %d records", len(create), len(del))
	dnsZone, err := c.Service.ReadWithContext(ctx, &dns.ReadRequest{ID: c.ZoneID})
	if err != nil {
		log.Printf("Error reading DNS zone before update: %v", err)
		return err
	}

	var newSets []*iaas.DNSRecord
	for _, rs := range dnsZone.Records {
		shouldDelete := false
		for _, dRec := range del {
			// Compare Type, Name, and RData (Targets[0]) for precise deletion
			if string(rs.Type) == dRec.Type && rs.Name == dRec.Name && rs.RData == dRec.Targets[0] {
				log.Printf("Deleting record: %s %s -> %v", dRec.Type, dRec.Name, dRec.Targets)
				shouldDelete = true
				break
			}
		}
		if !shouldDelete {
			newSets = append(newSets, rs)
		}
	}

	for _, cRec := range create {
		ttl := cRec.TTL
		if ttl == 0 {
			ttl = 3600 // fallback default
		}
		newRec := &iaas.DNSRecord{
			Type:  types.EDNSRecordType(cRec.Type),
			Name:  cRec.Name,
			RData: cRec.Targets[0],
			TTL:   ttl,
		}
		log.Printf("Creating record: %s %s -> %v (TTL=%d)", newRec.Type, newRec.Name, newRec.RData, newRec.TTL)
		newSets = append(newSets, newRec)
	}

	updateReq := &dns.UpdateRequest{
		ID:      c.ZoneID,
		Records: newSets,
		SettingsHash: dnsZone.SettingsHash, // Preserve existing settings hash
	}
	if _, err := c.Service.UpdateWithContext(ctx, updateReq); err != nil {
		log.Printf("Error applying DNS changes: %v", err)
		return err
	}
	log.Printf("DNS changes applied successfully")
	return nil
}

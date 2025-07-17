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
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"sigs.k8s.io/external-dns/endpoint"
)

// RecordsHandler handles GET /records requests.
// It retrieves all DNS records from SakuraCloud for the configured zone
// and returns them as a JSON array.
func RecordsHandler(client Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("[RecordsHandler] start zone=%s path=%s query=%s",
			client.GetZoneName(), r.URL.Path, r.URL.RawQuery)

		log.Printf("[RecordsHandler] GET /records invoked")

		// Fetch all records from SakuraCloud
		records, err := client.ListRecords()
		if err != nil {
			log.Printf("[RecordsHandler] error listing records: %v", err)
			http.Error(w, "failed to list DNS records", http.StatusInternalServerError)
			return
		}

		zoneSuffix := "." + client.GetZoneName()

		// Convert provider.Record to endpoint.Endpoint
		var endpoints []*endpoint.Endpoint
		for _, rec := range records {
			// Build the full DNS name (FQDN)
			fqdn := rec.Name
			if !strings.HasSuffix(fqdn, zoneSuffix) {
				fqdn += zoneSuffix
			}

			// Map SakuraCloud ALIAS into CNAME + alias flag
			epType := rec.Type
			if rec.Type == "ALIAS" {
				epType = "CNAME"
			}

			ep := &endpoint.Endpoint{
				DNSName:    fqdn,
				Targets:    rec.Targets,
				RecordType: epType,
				RecordTTL:  endpoint.TTL(rec.TTL),
			}

			// If original was ALIAS, attach providerSpecific alias=true
			if rec.Type == "ALIAS" {
				ep.ProviderSpecific = append(ep.ProviderSpecific,
					endpoint.ProviderSpecificProperty{
						Name:  "alias",
						Value: "true",
					},
				)
			}

			endpoints = append(endpoints, ep)
		}

		// Ensure we return an empty array, not null
		if endpoints == nil {
			endpoints = []*endpoint.Endpoint{}
		}

		w.Header().Set("Content-Type", "application/external.dns.webhook+json;version=1")
		if err := json.NewEncoder(w).Encode(endpoints); err != nil {
			log.Printf("[RecordsHandler] error encoding records to JSON: %v", err)
			http.Error(w, "failed to encode records to JSON", http.StatusInternalServerError)
			return
		}

		log.Printf("[RecordsHandler] successfully returned %d records (took %s)",
			len(endpoints), time.Since(start))
	}
}

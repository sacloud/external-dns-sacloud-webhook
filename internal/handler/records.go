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
	"context"
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

		ctx, cancel := context.WithTimeout(r.Context(), 10 * time.Second)
  		defer cancel()
		records, err := client.ListRecords(ctx)
		if err != nil {
			log.Printf("[RecordsHandler] error listing records: %v", err)
			http.Error(w, "failed to list DNS records", http.StatusInternalServerError)
			return
		}

		zoneSuffix := "." + client.GetZoneName()

		endpoints := []*endpoint.Endpoint{}
		for _, rec := range records {
			fqdn := rec.Name
			if !strings.HasSuffix(fqdn, zoneSuffix) {
				fqdn += zoneSuffix
			}

			epType := rec.Type
			providerSpecific := []endpoint.ProviderSpecificProperty{}
			if rec.Type == "ALIAS" {
				epType = "CNAME"
				providerSpecific = append(providerSpecific, endpoint.ProviderSpecificProperty{
					Name:  "alias",
					Value: "true",
				})
			}

			ep := &endpoint.Endpoint{
				DNSName:          fqdn,
				Targets:          rec.Targets,
				RecordType:       epType,
				RecordTTL:        endpoint.TTL(rec.TTL),
				ProviderSpecific: providerSpecific,
			}

			endpoints = append(endpoints, ep)
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

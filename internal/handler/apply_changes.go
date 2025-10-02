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
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/sacloud/external-dns-sacloud-webhook/internal/provider"
	"sigs.k8s.io/external-dns/endpoint"
)

// ChangeRequest defines the JSON payload for applyChanges requests.
// It carries the lists of endpoints to create and delete.
//
// NOTE: ExternalDNS webhook v1 may also send "updateOld" and "updateNew"
// for in-place updates such as TTL/target changes. To keep provider logic
// simple and idempotent, we convert updates into equivalent delete+create
// operations in the handler.
type ChangeRequest struct {
	Create    []*endpoint.Endpoint `json:"create"`
	Delete    []*endpoint.Endpoint `json:"delete"`
	UpdateOld []*endpoint.Endpoint `json:"updateOld"`
	UpdateNew []*endpoint.Endpoint `json:"updateNew"`
}

// safeTrimZoneSuffix trims the zone suffix only if the name actually ends
// with the given zone suffix. It also drops a trailing dot if any.
//
// This prevents accidental over-trimming when the input name does not belong
// to the current zone (or already came in relative form).
func safeTrimZoneSuffix(name, zoneSuffix string) string {
	if zoneSuffix != "" && strings.HasSuffix(name, zoneSuffix) {
		name = strings.TrimSuffix(name, zoneSuffix)
	}
	// Normalize trailing dot for safety (relative record names must not end with ".")
	return strings.TrimSuffix(name, ".")
}

// convertEndpoints converts []*endpoint.Endpoint to []provider.Record.
//
// - TXT ownership records: keep type TXT, strip surrounding quotes on targets.
// - CNAME/ALIAS: ensure targets end with a trailing dot (FQDN) for SakuraCloud.
// - TTL: use endpoint.RecordTTL if given (>0), otherwise fall back to 3600.
// - Name: convert to relative record name by trimming the zone suffix when present.
// - ALIAS: detected via providerSpecific "alias=true" on a CNAME endpoint.
//
func convertEndpoints(endpoints []*endpoint.Endpoint, zoneSuffix, txtPrefix string) []provider.Record {
	var records []provider.Record
	for _, e := range endpoints {
		if e == nil {
			continue
		}

		var recType string
		if e.RecordType == "TXT" && strings.HasPrefix(e.DNSName, txtPrefix) {
			// Always treat registry entries as TXT
			recType = "TXT"
		} else {
			// Otherwise, honor provided type and alias flag
			recType = e.RecordType
			if recType == "CNAME" {
				for _, ps := range e.ProviderSpecific {
					if ps.Name == "alias" && ps.Value == "true" {
						recType = "ALIAS"
						break
					}
				}
			}
		}

		// Convert to relative name with safe zone trimming
		name := safeTrimZoneSuffix(e.DNSName, zoneSuffix)

		// Normalize targets per record type
		targets := make([]string, 0, len(e.Targets))
		for _, t := range e.Targets {
			switch recType {
			case "TXT":
				// Remove surrounding quotes for TXT payload
				t = strings.Trim(t, "\"")
			case "CNAME", "ALIAS":
				// Ensure trailing dot for FQDN targets
				if !strings.HasSuffix(t, ".") {
					t += "."
				}
			}
			targets = append(targets, t)
		}

		ttl := 3600
		if e.RecordTTL > 0 {
			ttl = int(e.RecordTTL)
		}

		records = append(records, provider.Record{
			Type:    recType,
			Name:    name,
			Targets: targets,
			TTL:     ttl,
		})
	}
	return records
}

// ApplyHandler handles POST /records calls.
// It converts endpoint.Endpoints into provider.Record entries,
// strips the zone suffix from names, handles TXT quoting,
// ensures proper trailing dots for CNAME/ALIAS,
// and respects the "alias=true" providerSpecific flag.
//
// Additionally, this handler supports ExternalDNS "updateOld/updateNew" by
// projecting them to delete+create operations to keep the provider side simple.
func ApplyHandler(client Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[ApplyHandler] %s %s", r.Method, r.URL.Path)

		if ct := r.Header.Get("Content-Type"); ct != "application/external.dns.webhook+json;version=1" {
			log.Printf("[ApplyHandler] invalid Content-Type: %s", ct)
			http.Error(w, "invalid content type", http.StatusUnsupportedMediaType)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("[ApplyHandler] error reading body: %v", err)
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		log.Printf("[ApplyHandler] raw request body: %s", string(body))

		var req ChangeRequest
		if err := json.Unmarshal(body, &req); err != nil {
			log.Printf("[ApplyHandler] error decoding payload: %v", err)
			http.Error(w, "failed to decode request payload", http.StatusBadRequest)
			return
		}

		// Prepare suffix for trimming zone from DNS names
		zoneSuffix := "." + client.GetZoneName()
		// TXT registry prefix
		txtPrefix := "_external-dns."

		toCreate := convertEndpoints(req.Create, zoneSuffix, txtPrefix)
		toDelete := convertEndpoints(req.Delete, zoneSuffix, txtPrefix)

		// Convert updates into delete+create to surface them to the provider
		updateOld := convertEndpoints(req.UpdateOld, zoneSuffix, txtPrefix)
		updateNew := convertEndpoints(req.UpdateNew, zoneSuffix, txtPrefix)
		if len(updateOld) > 0 || len(updateNew) > 0 {
			toDelete = append(toDelete, updateOld...)
			toCreate = append(toCreate, updateNew...)
		}

		log.Printf("[ApplyHandler] create count: %d, delete count: %d (updateOld=%d, updateNew=%d)",
			len(toCreate), len(toDelete), len(req.UpdateOld), len(req.UpdateNew))

		if err := client.ApplyChanges(r.Context(), toCreate, toDelete); err != nil {
			log.Printf("[ApplyHandler] error applying changes: %v", err)
			http.Error(w, "failed to apply DNS changes", http.StatusInternalServerError)
			return
		}

		// On success, return 204 No Content
		w.Header().Set("Content-Type", "application/external.dns.webhook+json;version=1")
		w.WriteHeader(http.StatusNoContent)
		log.Printf("[ApplyHandler] successfully applied DNS changes")
	}
}

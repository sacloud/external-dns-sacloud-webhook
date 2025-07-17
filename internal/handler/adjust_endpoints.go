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

	"sigs.k8s.io/external-dns/endpoint"
)

// AdjustHandler handles POST /adjustendpoints requests.
// It accepts the desired endpoint set from controller, applies optional
// filtering or ownership logic, and returns the final endpoint set.
func AdjustHandler(client Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[AdjustHandler] POST /adjustendpoints invoked")

		if ct := r.Header.Get("Content-Type"); ct != "application/external.dns.webhook+json;version=1" {
			log.Printf("[AdjustHandler] invalid Content-Type: %s", ct)
			http.Error(w, "invalid content type", http.StatusUnsupportedMediaType)
			return
		}

		// Decode incoming desired endpoints
		var desired []*endpoint.Endpoint
		if err := json.NewDecoder(r.Body).Decode(&desired); err != nil {
			log.Printf("[AdjustHandler] error decoding payload: %v", err)
			http.Error(w, "failed to decode desired endpoints", http.StatusBadRequest)
			return
		}
		log.Printf("[AdjustHandler] received %d desired endpoints", len(desired))

		// Optionally, implement TXT registry filtering here (owner-id logic)
		// For now, we just pass through all desired endpoints
		adjusted := desired

		// Return final endpoints
		w.Header().Set(
			"Content-Type",
			"application/external.dns.webhook+json;version=1",
		)
		if err := json.NewEncoder(w).Encode(adjusted); err != nil {
			log.Printf("[AdjustHandler] error encoding response: %v", err)
			http.Error(w, "failed to encode adjusted endpoints", http.StatusInternalServerError)
			return
		}
		log.Printf("[AdjustHandler] returned %d endpoints", len(adjusted))
	}
}

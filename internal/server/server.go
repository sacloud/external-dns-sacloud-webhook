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
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/sacloud/external-dns-sacloud-webhook/internal/config"
	"github.com/sacloud/external-dns-sacloud-webhook/internal/handler"
	"github.com/sacloud/external-dns-sacloud-webhook/internal/provider"
)

// NewMux returns an http.ServeMux with all webhook routes registered.
func NewMux(client *provider.Client, cfg config.Config) *http.ServeMux {
	mux := http.NewServeMux()

	// Negotiation endpoint "/"
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Filter] %s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/external.dns.webhook+json;version=1")
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprintf(w, `{"domainFilter":["%s"],"recordTypes":["A","CNAME","TXT"]}`, cfg.ZoneName); err != nil {
			log.Printf("[Filter] write negotiation response failed: %v", err)
		}
	})

	// Health check "/healthz"
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Healthz] %s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/external.dns.webhook+json;version=1")
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprint(w, `{"status":"ok"}`); err != nil {
			log.Printf("[Healthz] write healthz response failed: %v", err)
		}
	})

	// Records listing & applying "/records"
	mux.HandleFunc("/records", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Records] %s %s", r.Method, r.URL.Path)
		switch r.Method {
		case http.MethodGet:
			handler.RecordsHandler(client)(w, r)
			log.Printf("[Records] GET /records invoked")
		case http.MethodPost:
			handler.ApplyHandler(client)(w, r)
			log.Printf("[Records] POST /records invoked")
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Adjust endpoints "/adjustendpoints"
	mux.HandleFunc("/adjustendpoints", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Adjust] %s %s", r.Method, r.URL.Path)
		handler.AdjustHandler(client)(w, r)
	})

	return mux
}

// Run initializes the client and starts the HTTP server.
func Run(cfg config.Config) {
	if cfg.ZoneName == "" {
		log.Fatal("[Server] ZONE environment variable is required")
	}
	log.Printf("[Server] Using DNS zone: %s", cfg.ZoneName)

	log.Printf("[Server] Initializing SakuraCloud DNS client")
	client, err := provider.NewClient(cfg.ZoneName, cfg.Token, cfg.Secret)
	if err != nil {
		log.Fatalf("[Server] Failed to create SakuraCloud client: %v", err)
	}

	if cfg.RegistryTXT {
		log.Printf("[Server] TXT registry enabled, owner ID: %s", cfg.TxtOwnerID)
	}

	mux := NewMux(client, cfg)
	addr := fmt.Sprintf("%s:%s", cfg.ProviderURL, cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	log.Printf("[Server] Starting webhook HTTP server at %s", addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("[Server] HTTP server error: %v", err)
	}
}

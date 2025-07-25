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
	"log"

	client "github.com/sacloud/api-client-go"
	iaas "github.com/sacloud/iaas-api-go"
	"github.com/sacloud/iaas-api-go/types"
	"github.com/sacloud/iaas-service-go/dns"
)

// ErrZoneNotFound is returned when the specified DNS zone cannot be found
var ErrZoneNotFound = errors.New("zone not found")

// Client manages DNS records for a specific SakuraCloud DNS zone
type Client struct {
	Context        context.Context // base context for API calls
	Service        DNSService      // underlying SakuraCloud DNS service
	ZoneName       string          // DNS zone name, e.g. "example.com"
	ZoneID         types.ID        // SakuraCloud DNS zone ID
}

// DNSService defines the methods used from the SakuraCloud DNS API
// including context-aware read/update for timeouts
type DNSService interface {
	FindWithContext(ctx context.Context, req *dns.FindRequest) ([]*iaas.DNS, error)
	ReadWithContext(ctx context.Context, req *dns.ReadRequest) (*iaas.DNS, error)
	UpdateWithContext(ctx context.Context, req *dns.UpdateRequest) (*iaas.DNS, error)
}

// NewClient initializes a SakuraCloud DNS client for the given zoneName
// token and secret must be provided. It also sets a default
// RequestTimeout of 10 seconds for API calls.
func NewClient(zoneName, token, secret string) (*Client, error) {
	log.Printf("Initializing SakuraCloud DNS client for zone '%s'", zoneName)

	opts := &client.Options{
		AccessToken:       token,
		AccessTokenSecret: secret,
		HttpRequestTimeout: 30,
		RetryWaitMax: 1,
	}
	apiClient := iaas.NewClientWithOptions(opts)
	log.Printf("SakuraCloud API client created with provided token, secret, and timeout")

	svc := dns.New(apiClient)
	log.Printf("SakuraCloud DNS service instance ready")

	log.Printf("Searching for DNS zone '%s'", zoneName)
	zones, err := svc.Find(&dns.FindRequest{})
	if err != nil {
		log.Printf("Error finding DNS zones: %v", err)
		return nil, err
	}

	var zoneID types.ID
	for _, z := range zones {
		log.Printf("Found zone: %s (ID: %d)", z.Name, z.ID)
		if z.Name == zoneName {
			zoneID = z.ID
			log.Printf("Matched target zone '%s' with ID %d", zoneName, zoneID)
			break
		}
	}
	if zoneID == 0 {
		log.Printf("Zone '%s' not found among %d zones", zoneName, len(zones))
		return nil, ErrZoneNotFound
	}

	client := &Client{
		Context:        context.Background(),
		Service:        svc,
		ZoneName:       zoneName,
		ZoneID:         zoneID,
	}
	log.Printf("Client for zone '%s' initialized successfully within http request timeout limit", zoneName)
	return client, nil
}

func (c *Client) GetZoneName() string {
	return c.ZoneName
}

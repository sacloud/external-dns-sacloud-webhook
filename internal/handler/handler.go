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

	"github.com/sacloud/external-dns-sacloud-webhook/internal/provider"
)

// To enable dependency injection, handlers use interface-based programming instead of concrete types.
type Provider interface {
	ListRecords(ctx context.Context) ([]provider.Record, error)
	ApplyChanges(ctx context.Context, create, delete []provider.Record) error
	GetZoneName() string
}

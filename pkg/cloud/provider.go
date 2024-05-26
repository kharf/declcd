// Copyright 2024 kharf
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cloud

import (
	"context"
	"errors"
	"net/http"
)

type ProviderID string

const (
	AWS   ProviderID = "aws"
	GCP   ProviderID = "gcp"
	Azure ProviderID = "azure"
)

var (
	ErrUnexpectedResponse = errors.New("Unexpected response")
)

// A Provider is a widely recognized cloud computing platform that provides several services for managing access and hosting containers.
type Provider interface {
	// FetchCredentials uses the configured provider identity and access management approach to receive temporary credentials for accessing cloud provider services, like container registries.
	FetchCredentials(context.Context) (*Credentials, error)
}

// GetProvider constructs a cloud Provider based on the given identifier or nil if no provider for given identifier could be constructed.
// Currently supported: gcp, aws, azure
func GetProvider(providerID ProviderID, host string, httpClient *http.Client) Provider {
	switch providerID {
	case GCP:
		return &GCPProvider{
			HttpClient: httpClient,
		}
	case AWS:
		return &AWSProvider{
			HttpClient: httpClient,
			Host:       host,
		}
	case Azure:
		return &AzureProvider{
			HttpClient: httpClient,
			Host:       host,
		}
	}

	return nil
}

// Temporary workload credentials used for cloud provider authentication and accessing cloud provider services.
type Credentials struct {
	Username string
	Password string
}

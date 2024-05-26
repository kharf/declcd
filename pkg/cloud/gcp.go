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
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	// Endpoint to the google metadata server, which provides access tokens.
	// See: https://cloud.google.com/compute/docs/access/authenticate-workloads
	GoogleMetadataServerTokenEndpoint = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"
)

// Access token for accessing google services like artifact registry.
type GoogleToken struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// GCPProvider is the dedicated provider for accessing Google Cloud services.
type GCPProvider struct {
	HttpClient *http.Client
}

var _ Provider = (*GCPProvider)(nil)

func (provider *GCPProvider) FetchCredentials(ctx context.Context) (*Credentials, error) {
	req, err := http.NewRequest(http.MethodGet, GoogleMetadataServerTokenEndpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Metadata-Flavor", "Google")

	response, err := provider.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"%w: got status code %d from gke metadata server",
			ErrUnexpectedResponse,
			response.StatusCode,
		)
	}

	var token GoogleToken
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		return nil, err
	}

	return &Credentials{
		Username: "oauth2accesstoken",
		Password: token.AccessToken,
	}, nil
}

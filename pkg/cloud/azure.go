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
	"io"
	"net/http"
	"net/url"
	"os"

	azureCloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// AzureProvider is the dedicated provider for accessing Azure cloud services.
type AzureProvider struct {
	HttpClient *http.Client
	Host       string
}

var _ Provider = (*AzureProvider)(nil)

type acrRefreshToken struct {
	RefreshToken string `json:"refresh_token"`
}

func (provider *AzureProvider) FetchCredentials(ctx context.Context) (*Credentials, error) {
	cred, err := azidentity.NewWorkloadIdentityCredential(
		&azidentity.WorkloadIdentityCredentialOptions{
			ClientOptions: policy.ClientOptions{
				Transport: provider.HttpClient,
			},
		},
	)
	if err != nil {
		return nil, err
	}

	azureADToken, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		// Client credential flows must have a scope value with /.default suffixed to the resource identifier
		Scopes: []string{
			fmt.Sprintf(
				"%s/.default",
				azureCloud.AzurePublic.Services[azureCloud.ResourceManager].Endpoint,
			),
		},
	})
	if err != nil {
		return nil, err
	}

	hostUrl, err := url.Parse(fmt.Sprintf("https://%s", provider.Host))
	if err != nil {
		return nil, err
	}

	data := url.Values{}
	data.Add("grant_type", "access_token")
	data.Add("service", hostUrl.Host)
	data.Add("tenant", os.Getenv("AZURE_TENANT_ID"))
	data.Add("access_token", azureADToken.Token)

	exchangeEndpoint := fmt.Sprintf("%s/oauth2/exchange", hostUrl.String())
	response, err := http.PostForm(exchangeEndpoint, data)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf(
			"%w: got status code %d from azure registry exchange endpoint %s: %s",
			ErrUnexpectedResponse,
			response.StatusCode,
			exchangeEndpoint,
			string(body),
		)
	}

	var refreshToken acrRefreshToken
	if err := json.NewDecoder(response.Body).Decode(&refreshToken); err != nil {
		return nil, err
	}

	return &Credentials{
		Username: "00000000-0000-0000-0000-000000000000",
		Password: refreshToken.RefreshToken,
	}, nil
}

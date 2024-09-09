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
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/kharf/declcd/pkg/kube"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	ErrSecretRefNotSet         = errors.New("Auth secret reference not set")
	ErrAuthSecretValueNotFound = errors.New("Auth secret value not found")
)

type ProviderID string

const (
	AWS   ProviderID = "aws"
	GCP   ProviderID = "gcp"
	Azure ProviderID = "azure"
)

// Workload credentials used for cloud provider authentication and accessing cloud provider services.
type Credentials struct {
	Username string
	Password string
}

// SecretRef is the reference to the secret containing the repository/registry authentication.
type SecretRef struct {
	Name string `json:"name"`
}

// WorkloadIdentity is a keyless approach used for repository/registry authentication.
type WorkloadIdentity struct {
	Provider ProviderID `json:"provider"`
}

// Auth contains methods for repository/registry authentication.
type Auth struct {
	SecretRef        *SecretRef        `json:"secretRef"`
	WorkloadIdentity *WorkloadIdentity `json:"workloadIdentity"`
}

var (
	ErrUnexpectedResponse = errors.New("Unexpected response")
)

// A Provider is a widely recognized cloud computing platform that provides several services for managing access and hosting containers.
type Provider interface {
	// FetchCredentials uses the configured provider identity and access management approach to receive credentials for accessing cloud provider services, like container registries.
	FetchCredentials(context.Context) (*Credentials, error)
}

// GetProvider constructs a cloud Provider based on the given identifier or nil if no provider for given identifier could be constructed.
// Currently supported: gcp, aws, azure
func GetProvider(
	providerID ProviderID,
	host url.URL,
	httpClient *http.Client,
	azureLoginURL string,
	gcpMetadataServerURL string,
) Provider {
	switch providerID {
	case GCP:
		return &GCPProvider{
			HttpClient:        httpClient,
			MetadataServerURL: gcpMetadataServerURL,
		}
	case AWS:
		return &AWSProvider{
			HttpClient: httpClient,
			URL:        host,
		}
	case Azure:
		return &AzureProvider{
			HttpClient: httpClient,
			URL:        host,
			LoginURL:   azureLoginURL,
		}
	}

	return nil
}

type options struct {
	httpClient           *http.Client
	namespace            string
	azureLoginURL        string
	gcpMetadataServerURL string
}

type option func(*options)

func WithHttpClient(client *http.Client) option {
	return func(o *options) {
		o.httpClient = client
	}
}

func WithNamespace(namespace string) option {
	return func(o *options) {
		o.namespace = namespace
	}
}

func WithCustomAzureLoginURL(url string) option {
	return func(o *options) {
		o.azureLoginURL = url
	}
}

func WithCustomGCPMetadataServerURL(url string) option {
	return func(o *options) {
		o.gcpMetadataServerURL = url
	}
}

func ReadCredentials(
	ctx context.Context,
	host string,
	auth Auth,
	kubeClient *kube.DynamicClient,
	opts ...option,
) (*Credentials, error) {
	options := options{
		httpClient: http.DefaultClient,
		namespace:  "default",
	}

	for _, opt := range opts {
		opt(&options)
	}

	if auth.WorkloadIdentity != nil {
		if strings.HasPrefix(host, "oci://") {
			host, _ = strings.CutPrefix(host, "oci://")
			host = fmt.Sprintf("https://%s", host)
		}

		if !strings.HasPrefix(host, "https://") && !strings.HasPrefix(host, "http://") {
			host = fmt.Sprintf("https://%s", host)
		}

		providerURL, err := url.Parse(host)
		if err != nil {
			return nil, err
		}

		provider := GetProvider(
			auth.WorkloadIdentity.Provider,
			*providerURL,
			options.httpClient,
			options.azureLoginURL,
			options.gcpMetadataServerURL,
		)

		return provider.FetchCredentials(ctx)
	}

	if auth.SecretRef == nil {
		return nil, ErrSecretRefNotSet
	}

	return readCredentialsFromSecret(
		ctx,
		auth.SecretRef.Name,
		options.namespace,
		kubeClient,
	)
}

func readCredentialsFromSecret(
	ctx context.Context,
	secretName string,
	namespace string,
	client *kube.DynamicClient,
) (*Credentials, error) {
	secretReq := &unstructured.Unstructured{}
	secretReq.SetKind("Secret")
	secretReq.SetAPIVersion("v1")
	secretReq.SetName(secretName)
	secretReq.SetNamespace(namespace)

	secret, err := client.Get(ctx, secretReq)
	if err != nil {
		return nil, err
	}

	data, found := secret.Object["data"].(map[string]interface{})
	var username, password string
	if found {
		username, err = getSecretValue(data, "username")
		if err != nil {
			return nil, err
		}
		password, err = getSecretValue(data, "password")
		if err != nil {
			return nil, err
		}
	} else {
		stringData, found := secret.Object["stringData"].(map[string]string)
		if !found {
			return nil, err
		}
		username = stringData["username"]
		password = stringData["password"]
	}

	return &Credentials{
		Username: username,
		Password: password,
	}, nil
}

func getSecretValue(data map[string]interface{}, key string) (string, error) {
	value := data[key]
	if value == nil {
		return "", fmt.Errorf("%w: %s is empty", ErrAuthSecretValueNotFound, key)
	}
	bytes, err := base64.StdEncoding.DecodeString(value.(string))
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

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
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/endpointcreds"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
)

// AWSProvider is the dedicated provider for accessing AWS services.
type AWSProvider struct {
	HttpClient *http.Client
	Host       string
}

var _ Provider = (*AWSProvider)(nil)

var (
	ErrUnexpectedHost = errors.New("Unexpected host")
)

func (provider *AWSProvider) FetchCredentials(ctx context.Context) (*Credentials, error) {
	hostParts := strings.Split(provider.Host, ".")
	if len(hostParts) != 6 {
		return nil, fmt.Errorf(
			"%w: expected AWS ecr host to be of format aws_account_id.dkr.ecr.region.amazonaws.com, got %s",
			ErrUnexpectedHost,
			provider.Host,
		)

	}

	config, err := config.LoadDefaultConfig(
		ctx,
		config.WithHTTPClient(provider.HttpClient),
		config.WithRegion(hostParts[3]),
		config.WithCredentialsProvider(
			endpointcreds.New(os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI")),
		),
	)
	if err != nil {
		return nil, err
	}

	client := ecr.NewFromConfig(config)
	tokenOutput, err := client.GetAuthorizationToken(ctx, nil)
	if err != nil {
		return nil, err
	}

	if len(tokenOutput.AuthorizationData) == 0 {
		return nil, fmt.Errorf("%w: got no authorization token from AWS ecr", ErrUnexpectedResponse)
	}

	authToken, err := base64.StdEncoding.DecodeString(
		*tokenOutput.AuthorizationData[0].AuthorizationToken,
	)
	if err != nil {
		return nil, err
	}

	tokenParts := strings.Split(string(authToken), ":")
	if len(tokenParts) != 2 {
		return nil, fmt.Errorf(
			"%w: decoded authorization token from AWS ecr is not of expected 'username:password' format",
			ErrUnexpectedResponse,
		)
	}

	return &Credentials{
		Username: tokenParts[0],
		Password: tokenParts[1],
	}, nil
}

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

package cloudtest

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/kharf/declcd/internal/ocitest"
	"github.com/kharf/declcd/pkg/cloud"
	"gotest.tools/v3/assert"
)

// A test Cloud Environment imitating either AWS, GCP or Azure Workload Identity auth.
type Environment interface {
	Close()
}

func NewCloudEnvironment(
	t testing.TB,
	provider cloud.ProviderID,
	registry *ocitest.Registry,
) Environment {
	switch provider {
	case cloud.GCP:
		return NewGCPEnvironment(t)
	case cloud.AWS:
		return NewAWSEnvironment(t, registry)
	case cloud.Azure:
		return NewAzureEnvironment(t, registry)
	}

	return nil
}

func newUnstartedServerFromEndpoint(
	t testing.TB,
	endpoint string,
	port string,
	mux *http.ServeMux,
) *httptest.Server {
	url, err := url.Parse(endpoint)
	assert.NilError(t, err)

	tcpAddr, err := net.ResolveTCPAddr("tcp", url.Hostname()+":"+port)
	assert.NilError(t, err)

	listener, err := net.ListenTCP("tcp", tcpAddr)
	assert.NilError(t, err)

	addr := url.Hostname() + ":" + port

	httpsServer := httptest.NewUnstartedServer(mux)
	httpsServer.Config.Addr = addr
	httpsServer.Listener = listener
	return httpsServer
}

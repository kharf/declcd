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
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"encoding/json"
	"github.com/kharf/declcd/pkg/cloud"
	"gotest.tools/v3/assert"
)

type Environment struct {
	HttpsServer *httptest.Server
	Client      *http.Client
}

func (env *Environment) Close() {
	if env.HttpsServer != nil {
		env.HttpsServer.Close()
	}
}

func StartCloudEnvironment(t testing.TB) *Environment {
	mux := http.NewServeMux()
	mux.HandleFunc(
		"GET /computeMetadata/v1/instance/service-accounts/default/token",
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			token := &cloud.GoogleToken{
				AccessToken: "aaaa",
				ExpiresIn:   10 * 60,
				TokenType:   "bearer",
			}
			err := json.NewEncoder(w).Encode(token)
			assert.NilError(t, err)
		},
	)

	googleEndpointURL, err := url.Parse(cloud.GoogleMetadataServerTokenEndpoint)
	assert.NilError(t, err)
	tcpAddr, err := net.ResolveTCPAddr("tcp", googleEndpointURL.Hostname()+":80")
	assert.NilError(t, err)
	listener, err := net.ListenTCP("tcp", tcpAddr)
	assert.NilError(t, err)
	addr := googleEndpointURL.Hostname() + ":80"

	httpsServer := httptest.NewUnstartedServer(mux)
	httpsServer.Config.Addr = addr
	httpsServer.Listener = listener
	httpsServer.Start()
	fmt.Println("Metadata Server listening on ", httpsServer.URL)

	client := httpsServer.Client()
	return &Environment{
		HttpsServer: httpsServer,
		Client:      client,
	}
}

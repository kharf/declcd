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
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"time"
)

// NewMetaServer creates an https server, which handles all requests destined for port 443.
func NewMetaServer(
	azureOidcIssuerUrl string,
) (*httptest.Server, error) {
	tlsMux := http.NewServeMux()
	tlsMux.HandleFunc(
		"POST /",
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			token := awsToken{
				AuthorizationData: []authorizationData{
					{
						AuthorizationToken: "ZGVjbGNkOmFiY2Q=",
						ExpiresAt:          time.Now().Add(10 * time.Minute).Unix(),
					},
				},
			}
			err := json.NewEncoder(w).Encode(&token)
			if err != nil {
				w.WriteHeader(500)
				return
			}
		},
	)
	tlsMux.HandleFunc(
		"GET /common/discovery/instance",
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			err := json.NewEncoder(w).Encode(&azureInstanceDiscoveryMetadata{
				TenantDiscoveryEndpoint: fmt.Sprintf(
					"%s/%s/v2.0/.well-known/openid-configuration",
					azureOidcIssuerUrl,
					"tenant",
				),
			})
			if err != nil {
				w.WriteHeader(500)
				return
			}
		},
	)

	tlsServer, err := newUnstartedServerFromEndpoint("443", tlsMux)
	if err != nil {
		return nil, err
	}
	tlsServer.StartTLS()
	fmt.Println("TLS Meta Server listening on", tlsServer.URL)

	return tlsServer, nil
}

func newUnstartedServerFromEndpoint(
	port string,
	mux *http.ServeMux,
) (*httptest.Server, error) {
	addr := "127.0.0.1:" + port
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}

	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return nil, err
	}

	httpServer := httptest.NewUnstartedServer(mux)
	httpServer.Config.Addr = addr
	httpServer.Listener = listener
	return httpServer, nil
}

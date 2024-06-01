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
	"net/http"
	"net/http/httptest"

	"github.com/kharf/declcd/pkg/cloud"
)

// A test Cloud Environment imitating GCP Metadata Server.
type GCPEnvironment struct {
	HttpsServer *httptest.Server
}

func (env *GCPEnvironment) Close() {
	if env.HttpsServer != nil {
		env.HttpsServer.Close()
	}
}

func NewGCPEnvironment() (*GCPEnvironment, error) {
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
			if err != nil {
				w.WriteHeader(500)
				return
			}
		},
	)

	httpServer, err := newUnstartedServerFromEndpoint(
		"80",
		mux,
	)
	if err != nil {
		return nil, err
	}
	httpServer.Start()

	fmt.Println("Metadata Server listening on", httpServer.URL)

	return &GCPEnvironment{
		HttpsServer: httpServer,
	}, nil
}

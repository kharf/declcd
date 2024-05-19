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
	"testing"

	"github.com/kharf/declcd/pkg/cloud"
	"gotest.tools/v3/assert"
)

// A test Cloud Environment imitating GCP Metadata Server.
type GCPEnvironment struct {
	HttpsServer *httptest.Server
}

var _ Environment = (*GCPEnvironment)(nil)

func (env *GCPEnvironment) Close() {
	if env.HttpsServer != nil {
		env.HttpsServer.Close()
	}
}

func NewGCPEnvironment(t testing.TB) *GCPEnvironment {
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

	httpsServer := newUnstartedServerFromEndpoint(
		t,
		cloud.GoogleMetadataServerTokenEndpoint,
		"80",
		mux,
	)
	httpsServer.Start()
	fmt.Println("Metadata Server listening on", httpsServer.URL)

	return &GCPEnvironment{
		HttpsServer: httpsServer,
	}
}

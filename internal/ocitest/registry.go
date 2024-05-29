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

package ocitest

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"cuelabs.dev/go/oci/ociregistry/ocimem"
	"cuelabs.dev/go/oci/ociregistry/ociserver"
	"cuelang.org/go/mod/modregistry"
	"cuelang.org/go/mod/module"
	"cuelang.org/go/mod/modzip"
	"gotest.tools/v3/assert"

	"github.com/kharf/declcd/pkg/cloud"
	"github.com/otiai10/copy"
)

type Registry struct {
	httpsServer    *httptest.Server
	client         *http.Client
	registryClient *modregistry.Client
}

func (r *Registry) Client() *http.Client {
	return r.client
}

func (r *Registry) OCIClient() *modregistry.Client {
	return r.registryClient
}

func (r *Registry) Addr() string {
	return r.httpsServer.Config.Addr
}

func (r *Registry) URL() string {
	return r.httpsServer.URL
}

func (r *Registry) Close() {
	if r.httpsServer != nil {
		r.httpsServer.Close()
	}
}

// Creates an OCI registry to test tls/https.
//
// Note: Helm uses Docker under the hood to handle OCI
// and Docker defaults to HTTP when it detects that the registry host
// is localhost or 127.0.0.1.
// In order to test OCI with a HTTPS server, we have to supply a "fake" host.
// We use a mock dns server to create an A record which binds declcd.io to 127.0.0.1.
// All OCI tests have to use declcd.io as host.
func NewTLSRegistry(t testing.TB, private bool, cloudProviderID string) (*Registry, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", "declcd.io:0")
	if err != nil {
		return nil, err
	}
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return nil, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	addr := "declcd.io:" + strconv.Itoa(port)
	registry := ocimem.New()
	ociHandler := ociserver.New(registry, nil)
	mux := http.NewServeMux()
	mux.HandleFunc(
		"POST /oauth2/exchange",
		func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			assert.NilError(t, err)
			assert.Equal(
				t,
				string(body),
				"access_token=nottheacrtoken&grant_type=access_token&service=declcd.io%3A"+strconv.Itoa(
					port,
				)+"&tenant=tenant",
			)

			w.WriteHeader(200)
			_, err = w.Write([]byte(`{"refresh_token": "aaaa"}`))
			assert.NilError(t, err)
		},
	)
	mux.HandleFunc(
		"/v2/",
		func(w http.ResponseWriter, r *http.Request) {
			if private {
				if r.URL.Path == "/v2/" {
					auth, found := r.Header["Authorization"]
					if !found {
						w.Header().Set("WWW-Authenticate", "Basic realm=\"test\"")
						w.WriteHeader(401)
						return
					}
					assert.Assert(t, found)
					assert.Assert(t, len(auth) == 1)
					credsBase64, found := strings.CutPrefix(auth[0], "Basic ")
					assert.Assert(t, found)
					credsBytes, err := base64.StdEncoding.DecodeString(credsBase64)
					assert.NilError(t, err)
					creds := string(credsBytes)

					var expectedCreds string
					switch cloudProviderID {
					case string(cloud.GCP):
						expectedCreds = "oauth2accesstoken:aaaa"
					case string(cloud.Azure):
						expectedCreds = "00000000-0000-0000-0000-000000000000:aaaa"
					default:
						expectedCreds = "declcd:abcd"
					}
					assert.Equal(t, creds, expectedCreds)
				}
			}

			ociHandler.ServeHTTP(w, r)
		},
	)
	httpsServer := httptest.NewUnstartedServer(mux)
	httpsServer.Config.Addr = addr
	httpsServer.Listener = listener
	httpsServer.StartTLS()

	fmt.Println("TLS Registry listening on", httpsServer.URL)
	client := httpsServer.Client()

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client.Transport = transport
	ociClient := modregistry.NewClient(registry)

	httpsServer.URL = strings.Replace(
		httpsServer.URL,
		"https://127.0.0.1",
		"oci://declcd.io",
		1,
	)

	return &Registry{
		httpsServer:    httpsServer,
		client:         client,
		registryClient: ociClient,
	}, nil
}

func StartCUERegistry(
	t testing.TB,
	testRoot string,
) *Registry {
	cueModuleRegistry, err := NewTLSRegistry(t, false, "")
	assert.NilError(t, err)
	ociClient := cueModuleRegistry.OCIClient()
	modDir, err := os.MkdirTemp(testRoot, "")
	assert.NilError(t, err)
	schemaSrc := "schema"
	err = copy.Copy(schemaSrc, filepath.Join(modDir, schemaSrc))
	assert.NilError(t, err)
	ctx := context.Background()
	m, err := module.NewVersion("github.com/kharf/declcd/schema", "v0.9.1")
	assert.NilError(t, err)
	schemaModuleReader, schemaLen := createImage(t, m, modDir, "schema")
	err = ociClient.PutModule(ctx, m, schemaModuleReader, schemaLen)
	assert.NilError(t, err)
	modDirSrc := "test/mod/cue/k8s"
	err = copy.Copy(modDirSrc, filepath.Join(modDir, "k8s"))
	assert.NilError(t, err)
	m, err = module.NewVersion("github.com/kharf/cuepkgs/modules/k8s", "v0.0.5")
	assert.NilError(t, err)
	cuepkgsModuleReader, cuepkgsLen := createImage(t, m, modDir, "k8s")
	err = ociClient.PutModule(ctx, m, cuepkgsModuleReader, cuepkgsLen)
	assert.NilError(t, err)
	err = ociClient.PutModule(ctx, m, cuepkgsModuleReader, int64(cuepkgsLen))
	assert.NilError(t, err)
	http.DefaultClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	err = os.Setenv("CUE_REGISTRY", cueModuleRegistry.Addr())
	assert.NilError(t, err)
	return cueModuleRegistry
}

func createImage(
	t testing.TB,
	m module.Version,
	modDir string,
	mod string,
) (io.ReaderAt, int64) {
	zipFile, err := createZip(t, m, modDir, mod)
	assert.NilError(t, err)
	return bytes.NewReader(zipFile), int64(len(zipFile))
}

func createZip(
	t testing.TB,
	m module.Version,
	modDir string,
	mod string,
) ([]byte, error) {
	var zipBytes bytes.Buffer
	err := modzip.CreateFromDir(&zipBytes, m, filepath.Join(modDir, mod))
	assert.NilError(t, err)
	return zipBytes.Bytes(), nil
}

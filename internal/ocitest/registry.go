package ocitest

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"cuelabs.dev/go/oci/ociregistry/ocimem"
	"cuelabs.dev/go/oci/ociregistry/ociserver"
	"cuelang.org/go/mod/modregistry"
	"cuelang.org/go/mod/module"
	"cuelang.org/go/mod/modzip"
	"gotest.tools/v3/assert"

	"github.com/foxcpp/go-mockdns"
	"github.com/otiai10/copy"
)

type Registry struct {
	httpsServer    *httptest.Server
	client         *http.Client
	dnsServer      *mockdns.Server
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
	if r.dnsServer != nil {
		r.dnsServer.Close()
	}
}

func NewTLSRegistry() (*Registry, error) {
	// Helm uses Docker under the hood to handle OCI
	// and Docker defaults to HTTP when it detects that the registry host
	// is localhost or 127.0.0.1.
	// In order to test OCI with a HTTPS server, we have to supply a "fake" host.
	// We use a mock dns server to create an A record which binds declcd.io to 127.0.0.1.
	// All OCI tests have to use declcd.io as host.
	dnsServer, err := mockdns.NewServer(map[string]mockdns.Zone{
		"declcd.io.": {
			A: []string{"127.0.0.1"},
		},
	}, false)
	if err != nil {
		return nil, err
	}
	dnsServer.PatchNet(net.DefaultResolver)
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
	httpsServer := httptest.NewUnstartedServer(ociserver.New(registry, nil))
	httpsServer.Config.Addr = addr
	httpsServer.Listener = listener
	httpsServer.StartTLS()
	client := httpsServer.Client()
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client.Transport = transport
	// set to to true globally as CUE for example uses the DefaultTransport
	http.DefaultTransport = transport
	ociClient := modregistry.NewClient(registry)
	return &Registry{
		httpsServer:    httpsServer,
		client:         client,
		dnsServer:      dnsServer,
		registryClient: ociClient,
	}, nil
}

func StartCUERegistry(
	t testing.TB,
	testRoot string,
) *Registry {
	cueModuleRegistry, err := NewTLSRegistry()
	assert.NilError(t, err)
	ociClient := cueModuleRegistry.OCIClient()
	modDir, err := os.MkdirTemp(testRoot, "")
	assert.NilError(t, err)
	modDirSrc := "test/mod/cue/"
	err = copy.Copy(modDirSrc, modDir)
	assert.NilError(t, err)
	ctx := context.Background()
	m, err := module.NewVersion("github.com/kharf/declcd/schema", "v0.9.1")
	assert.NilError(t, err)
	schemaModuleReader, schemaLen := createImage(t, m, modDir, "schema")
	err = ociClient.PutModule(ctx, m, schemaModuleReader, schemaLen)
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

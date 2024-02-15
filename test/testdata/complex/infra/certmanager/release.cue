package certmanager

import (
	"github.com/kharf/declcd/api/v1"
)

_release: v1.#HelmRelease & {
	name:      "cert-manager"
	namespace: _namespace.metadata.name
	chart: {
		name:    "cert-manager"
		repoURL: "oci://registry-1.docker.io/bitnamicharts"
		version: "0.18.0"
	}
	values: {
		installCRDs: true
	}
}

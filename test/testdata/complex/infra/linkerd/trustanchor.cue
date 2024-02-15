package linkerd

import "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"

_trustAnchor: v1.#Issuer & {
	apiVersion: "cert-manager.io/v1"
	kind:       "Issuer"
	metadata: {
		name:      "linkerd-trust-anchor"
		namespace: _namespace.metadata.name
	}
	spec: ca: secretName: "linkerd-trust-anchor"
}

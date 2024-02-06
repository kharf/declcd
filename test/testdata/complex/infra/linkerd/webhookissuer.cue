package linkerd

import "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"

_webhookIssuer: v1.#Issuer & {
	apiVersion: "cert-manager.io/v1"
	kind:       "Issuer"
	metadata: {
		name:      "webhook-issuer"
		namespace: _namespace.metadata.name
	}
	spec: ca: secretName: "webhook-issuer-tls"
}

package linkerd

import "github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/core/v1"

_trustAnchorSecret: v1.#Secret & {
	apiVersion: "v1"
	stringData: {
		"tls.crt": """
			"""
		"tls.key": """
			"""
	}
	kind: "Secret"
	metadata: {
		name:      "linkerd-trust-anchor"
		namespace: _namespace.metadata.name
	}
	type: "kubernetes.io/tls"
}

_webhookIssuerSecret: v1.#Secret & {
	apiVersion: "v1"
	stringData: {
		"tls.crt": """
			"""
		"tls.key": """
			"""
	}
	kind: "Secret"
	metadata: {
		name:      "webhook-issuer-tls"
		namespace: _namespace.metadata.name
	}
	type: "kubernetes.io/tls"
}

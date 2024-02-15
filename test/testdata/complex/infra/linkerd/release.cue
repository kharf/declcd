package linkerd

import (
	"github.com/kharf/declcd/api/v1"
)

_crdsRelease: v1.#HelmRelease & {
	name:      "linkerd-crds"
	namespace: _namespace.metadata.name
	chart: {
		name:    "linkerd-crds"
		repoURL: "https://helm.linkerd.io/stable"
		version: "1.8.0"
	}
}

_controlPlaneRelease: v1.#HelmRelease & {
	name:      "linkerd-control-plane"
	namespace: _namespace.metadata.name
	chart: {
		name:    "linkerd-control-plane"
		repoURL: "https://helm.linkerd.io/stable"
		version: "1.16.8"
	}
	values: {
		_webhookCert: _webhookIssuerSecret.stringData."tls.crt"
		identity: issuer: scheme: "kubernetes.io/tls"
		identityTrustAnchorsPEM: _trustAnchorSecret.stringData."tls.crt"
		proxyInjector: {
			externalSecret: true
			caBundle:       _webhookCert
		}
		profileValidator: {
			externalSecret: true
			caBundle:       _webhookCert
		}
		policyValidator: {
			externalSecret: true
			caBundle:       _webhookCert
		}
	}
}

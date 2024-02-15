package linkerd

import "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"

_identityIssuer: v1.#Certificate & {
	apiVersion: "cert-manager.io/v1"
	kind:       "Certificate"
	metadata: {
		name:      "linkerd-identity-issuer"
		namespace: _namespace.metadata.name
	}
	spec: {
		secretName:  "linkerd-identity-issuer"
		duration:    "48h"
		renewBefore: "25h"
		issuerRef: {
			name: "linkerd-trust-anchor"
			kind: "Issuer"
		}
		commonName: "identity.linkerd.cluster.local"
		dnsNames: ["identity.linkerd.cluster.local"]
		isCA: true
		privateKey: algorithm: "ECDSA"
		usages: [
			"cert sign",
			"crl sign",
			"server auth",
			"client auth",
		]
	}
}

_proxyInjector: v1.#Certificate & {
	apiVersion: "cert-manager.io/v1"
	kind:       "Certificate"
	metadata: {
		name:      "linkerd-proxy-injector"
		namespace: _namespace.metadata.name
	}
	spec: {
		secretName:  "linkerd-proxy-injector-k8s-tls"
		duration:    "24h"
		renewBefore: "1h"
		issuerRef: {
			name: "webhook-issuer"
			kind: "Issuer"
		}
		commonName: "linkerd-proxy-injector.linkerd.svc"
		dnsNames: ["linkerd-proxy-injector.linkerd.svc"]
		isCA: false
		privateKey: algorithm: "ECDSA"
		usages: ["server auth"]
	}
}

_spValidator: v1.#Certificate & {
	apiVersion: "cert-manager.io/v1"
	kind:       "Certificate"
	metadata: {
		name:      "linkerd-sp-validator"
		namespace: _namespace.metadata.name
	}
	spec: {
		secretName:  "linkerd-sp-validator-k8s-tls"
		duration:    "24h"
		renewBefore: "1h"
		issuerRef: {
			name: "webhook-issuer"
			kind: "Issuer"
		}
		commonName: "linkerd-sp-validator.linkerd.svc"
		dnsNames: ["linkerd-sp-validator.linkerd.svc"]
		isCA: false
		privateKey: algorithm: "ECDSA"
		usages: ["server auth"]
	}
}

_policyValidator: v1.#Certificate & {
	apiVersion: "cert-manager.io/v1"
	kind:       "Certificate"
	metadata: {
		name:      "linkerd-policy-validator"
		namespace: _namespace.metadata.name
	}
	spec: {
		secretName:  "linkerd-policy-validator-k8s-tls"
		duration:    "24h"
		renewBefore: "1h"
		issuerRef: {
			name: "webhook-issuer"
			kind: "Issuer"
		}
		commonName: "linkerd-policy-validator.linkerd.svc"
		dnsNames: ["linkerd-policy-validator.linkerd.svc"]
		isCA: false
		privateKey: {
			algorithm: "ECDSA"
			encoding:  "PKCS8"
		}
		usages: ["server auth"]
	}
}

package linkerd

import (
	"github.com/kharf/declcd/api/v1"
	"github.com/kharf/decldc-test-repo/infra/certmanager"
)

#Linkerd: v1.#Component & {
	dependencies: [
		certmanager.ns.id,
	]
}
[Name=_]: #Linkerd

ns: content:                  _namespace
trustAnchorSecret: content:   _trustAnchorSecret
webhookIssuerSecret: content: _webhookIssuerSecret
identityIssuer: content:      _identityIssuer
proxyInjector: content:       _proxyInjector
spValidator: content:         _spValidator
policyValidator: content:     _policyValidator
trustAnchor: content:         _trustAnchor
webhookIssuer: content:       _webhookIssuer
crdsRelease: content:         _crdsRelease
controlPlaneRelease: content: _controlPlaneRelease

package linkerd

import (
	"github.com/kharf/declcd/api/v1"
	"github.com/kharf/decldc-test-repo/infra/certmanager"
)

v1.#Component & {
	linkerd: {
		dependencies: [
			certmanager.certManager.id,
		]
		manifests: [
			_namespace,
			_trustAnchorSecret,
			_webhookIssuerSecret,
			_identityIssuer,
			_proxyInjector,
			_spValidator,
			_policyValidator,
			_trustAnchor,
			_webhookIssuer,
		]
		helmReleases: [
			_crdsRelease,
			_controlPlaneRelease,
		]
	}
}

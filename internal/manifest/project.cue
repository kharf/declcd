package declcd

import (
	"github.com/kharf/declcd/schema/component"
)

{{.Name}}: component.#Manifest & {
	dependencies: [
		crd.id,
		ns.id,
	]
	content: {
		apiVersion: "gitops.declcd.io/v1beta1"
		kind:       "GitOpsProject"
		metadata: {
			name:      "{{.Name}}"
			namespace: "{{.Namespace}}"
			labels: _{{.Shard}}Labels
		}
		spec: {
			branch:              "{{.Branch}}"
			pullIntervalSeconds: {{.PullIntervalSeconds}}
			suspend:             false
			url:                 "{{.Url}}"
		}
	}
}

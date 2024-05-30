package declcd

import (
	"github.com/kharf/declcd/schema/component"
)

_projectName: "{{.Name}}"

project: component.#Manifest & {
	dependencies: [crd.id]
	content: {
		apiVersion: "gitops.declcd.io/v1beta1"
		kind:       "GitOpsProject"
		metadata: {
			name:      _projectName
			namespace: "{{.Namespace}}"
		}
		spec: {
			branch:              "{{.Branch}}"
			pullIntervalSeconds: {{.PullIntervalSeconds}}
			name:                _projectName
			suspend:             false
			url:                 "{{.Url}}"
		}
	}
}

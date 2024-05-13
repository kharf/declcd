package schema

import "strings"

#Manifest: {
	type:          "Manifest"
	_groupVersion: strings.Split(content.apiVersion, "/")
	_group:        string | *""
	if len(_groupVersion) >= 2 {
		_group: _groupVersion[0]
	}
	id: "\(content.metadata.name)_\(content.metadata.namespace)_\(_group)_\(content.kind)"
	dependencies: [...string]
	content: {
		apiVersion!: string & strings.MinRunes(1)
		kind!:       string & strings.MinRunes(1)
		metadata: {
			namespace: string | *""
			name!:     string & strings.MinRunes(1)
			...
		}
		...
	}
}

#HelmRelease: {
	type: "HelmRelease"
	id:   "\(name)_\(namespace)_\(type)"
	dependencies: [...string]
	name!:      string & strings.MinRunes(1)
	namespace!: string
	chart!:     #HelmChart
	values: {...}
}

#HelmChart: {
	name!:    string & strings.MinRunes(1)
	repoURL!: string & strings.HasPrefix("oci://") | strings.HasPrefix("http://") | strings.HasPrefix("https://")
	version!: string & strings.MinRunes(1)
	auth?:    #Auth
}

#Auth: {
	workloadIdentity: {
		provider: "gcp" | "aws" | "azure"
	}
} | {
	secretRef: {
		name:      string & strings.MinRunes(1)
		namespace: string & strings.MinRunes(1)
	}
}

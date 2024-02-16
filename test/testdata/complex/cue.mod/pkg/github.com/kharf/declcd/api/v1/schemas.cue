package v1

import "strings"

#Component: {
	if content._type == "HelmRelease" {
		id: "\(content.name)_\(content.namespace)_\(content._type)"
	}
	if content._type == "Manifest" {
		_groupVersion: strings.Split(content.apiVersion, "/")
		_group:        string | *""
		if len(_groupVersion) >= 2 {
			_group: _groupVersion[0]
		}
		id: "\(content.metadata.name)_\(content.metadata.namespace)_\(_group)_\(content.kind)"
	}
	type:     content._type
	content!: #Manifest | #HelmRelease
	dependencies: [...string]
}

[Name=_]: #Component

#Manifest: {
	_type:       "Manifest"
	apiVersion!: string
	kind!:       string
	metadata: {
		namespace: string | *""
		name!:     string
		...
	}
	...
}

#HelmChart: {
	name!:    string
	repoURL!: string
	version!: string
}

#HelmRelease: {
	_type:      "HelmRelease"
	name!:      string
	namespace!: string
	chart!:     #HelmChart
	values: {...}
}

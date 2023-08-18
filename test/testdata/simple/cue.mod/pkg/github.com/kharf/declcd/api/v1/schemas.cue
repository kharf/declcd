package v1

// Defines the CUE schema of decl's components.
#Component: {
	intervalSeconds: uint | *60
	manifests: [...]
	helmReleases: [...#HelmRelease]
}

#HelmChart: {
	name:    string
	repoURL: string
	version: string
}

// Defines the CUE schema of decl's HelmRelease type.
#HelmRelease: {
	name:      string
	namespace: string
	chart:     #HelmChart
	values: {...}
}

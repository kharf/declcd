package v1

// Defines the CUE schema of decl's components.
#Component: {
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

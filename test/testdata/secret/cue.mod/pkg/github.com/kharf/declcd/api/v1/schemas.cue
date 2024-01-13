package v1

#Component: [Name=_]: {
	id: Name
	dependencies: [...string]
	manifests: [...]
	helmReleases: [...#HelmRelease]
}

#HelmChart: {
	name:    string
	repoURL: string
	version: string
}

#HelmRelease: {
	name:      string
	namespace: string
	chart:     #HelmChart
	values: {...}
}

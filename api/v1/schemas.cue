package v1

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

#HelmRelease: {
	name:      string
	namespace: string
	chart:     #HelmChart
	values: {...}
}

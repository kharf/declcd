package releasenamespacemissing

release: {
	type: "HelmRelease"
	content: {
		name: "{{.Name}}"
		chart: {
			name:    "test"
			repoURL: "{{.RepoUrl}}"
			version: "{{.Version}}"
		}
		values: {
			autoscaling: enabled: true
		}
	}
}

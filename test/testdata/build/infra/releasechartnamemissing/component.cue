package releasechartnamemissing

release: {
	type: "HelmRelease"
	content: {
		namespace: "test"
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

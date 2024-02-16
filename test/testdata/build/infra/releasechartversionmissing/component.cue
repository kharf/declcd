package releasechartversionmissing

release: {
	type: "HelmRelease"
	content: {
		name:      "{{.Name}}"
		namespace: "test"
		chart: {
			name:    "test"
			repoURL: "{{.RepoUrl}}"
		}
		values: {
			autoscaling: enabled: true
		}
	}
}

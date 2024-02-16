package releasechartrepourlmissing

release: {
	type: "HelmRelease"
	content: {
		name:      "{{.Name}}"
		namespace: "test"
		chart: {
			name:    "test"
			version: "{{.Version}}"
		}
		values: {
			autoscaling: enabled: true
		}
	}
}

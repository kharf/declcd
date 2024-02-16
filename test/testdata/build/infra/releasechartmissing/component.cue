package releasechartmissing

release: {
	type: "HelmRelease"
	content: {
		name:      "{{.Name}}"
		namespace: "test"
		values: {
			autoscaling: enabled: true
		}
	}
}

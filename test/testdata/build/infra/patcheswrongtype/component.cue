package patcheswrongtype

release: {
	type: "HelmRelease"
	id:   "unimportant"
	dependencies: []
	name:      "test"
	namespace: "test"
	chart: {
		name:    "test"
		repoURL: "oci://test"
		version: "test"
	}

	patches: [
		"hello",
	]

	values: {}
}

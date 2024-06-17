package kindmissing

secret: {
	type: "Manifest"
	content: {
		apiVersion: "v1"
		metadata: {
			name:      "secret"
			namespace: "test"
		}
		data: {
			foo: 'bar'
		}
	}
}

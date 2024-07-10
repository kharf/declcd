package kindmissing

secret: {
	type: "Manifest"
	id:   "unimportant"
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

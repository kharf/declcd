package apiversionmissing

secret: {
	type: "Manifest"
	id:   "unimportant"
	content: {
		kind: "Secret"
		metadata: {
			name:      "secret"
			namespace: "test"
		}
		data: {
			foo: 'bar'
		}
	}
}

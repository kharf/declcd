package apiversionmissing

secret: {
	type: "Manifest"
	content: {
		kind: "Secret"
		metadata: {
			name:      "secret"
			namespace: "test"
		}
		data: {
			foo: '(enc;value omitted)'
		}
	}
}

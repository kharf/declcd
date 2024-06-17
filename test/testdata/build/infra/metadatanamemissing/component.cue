package metadatanamemissing

secret: {
	type: "Manifest"
	content: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			namespace: "test"
		}
		data: {
			foo: 'bar'
		}
	}
}

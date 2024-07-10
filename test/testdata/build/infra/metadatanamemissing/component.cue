package metadatanamemissing

secret: {
	type: "Manifest"
	id:   "unimportant"
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

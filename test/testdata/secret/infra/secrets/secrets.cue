package secrets

import "k8s.io/api/core/v1"

#data: v1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      "secret"
		namespace: #namespace.metadata.name
	}
	data: {
		foo: 'bar'
	}
}

#stringData: v1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      "secret"
		namespace: #namespace.metadata.name
	}
	stringData: {
		foo: "bar"
	}
}

#both: v1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      "secret"
		namespace: #namespace.metadata.name
	}
	stringData: {
		foo: "bar"
	}
	data: {
		foo: 'bar'
	}
}

#multiLine: v1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      "secret"
		namespace: #namespace.metadata.name
	}
	stringData: {
		foo: """
				bar
				bar
				bar
			"""
	}
	data: {
		foo: '''
				bar
				bar
				bar
			'''
	}
}

#none: v1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      "secret"
		namespace: #namespace.metadata.name
	}
}

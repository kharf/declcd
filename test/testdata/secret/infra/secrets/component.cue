package secrets

import (
	"github.com/kharf/declcd/schema@v0"
	corev1 "github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/core/v1"
)

#Namespace: {
	_name!: string
	schema.#Manifest & {
		content: corev1.#Namespace & {
			apiVersion: "v1"
			kind:       "Namespace"
			metadata: {
				name: _name
			}
		}
	}
}

ns: #Namespace & {
	_name: "mynamespace"
}

#Secret: {
	_name!: string
	data: {[string]: bytes}
	stringData: {[string]: string}
	corev1.#Secret & {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      _name
			namespace: ns.content.metadata.name
		}
	}
}

b: #Secret & {
	_name: "b"
	stringData: {
		foo: _bSecret
	}
}

c: #Secret & {
	_name: "c"
	data: {
		foo: _cSecret
	}
}

data: #Secret & {
	_name: "data"
	data: {
		foo: 'bar'
	}
}

stringData: #Secret & {
	_name: "stringData"
	stringData: {
		foo: "bar"
	}
}

both: #Secret & {
	_name: "both"
	stringData: {
		foo: "bar"
	}
	data: {
		foo: 'bar'
	}
}

multiLine: #Secret & {
	_name: "multiLine"
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

none: #Secret & {
	_name: "none"
}

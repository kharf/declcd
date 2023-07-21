package podinfo

component: {
	intervalSeconds: 60
	manifests: [
		_namespace,
		_deployment,
	]
}
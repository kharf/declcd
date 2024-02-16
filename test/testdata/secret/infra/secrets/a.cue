package secrets

_fooSecret: 'bar'
a: #Secret & {
	_name: "a"
	data: {
		foo: _fooSecret
	}
}

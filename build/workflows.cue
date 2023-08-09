package build

import "json.schemastore.org/github"

#workflow: {
	_name:    string
	workflow: github.#Workflow & {
		name:        _name
		permissions: "read-all"
		jobs: [string]: {
			"runs-on": "ubuntu-latest"
			steps: [
				#checkoutCode,
				...,
			]
		}
	}
	...
}

#checkoutCode: {
	name: "Checkout code"
	uses: "actions/checkout@v3.5.3"
	with: {
		token: "${{ secrets.PAT }}"
	}
}

#setupGo: {
	name: "Setup Go"
	uses: "actions/setup-go@v4"
	with: {
		"go-version-file":       "build/go.mod"
		"check-latest":          true
		cache:                   true
		"cache-dependency-path": "build/go.sum"
	}
}

#pipeline: {
	name:                string
	run:                 string
	"working-directory": "./build"
}

workflows: [
	#workflow & {
		_name:    "pr-verification"
		workflow: github.#Workflow & {
			on: {
				pull_request: {
					branches: [
						"*",
					]
					"tags-ignore": [
						"*",
					]
				}
			}

			jobs: "\(_name)": {
				steps: [
					#checkoutCode,
					#setupGo,
					#pipeline & {
						name: "Verification Pipeline"
						run:  "go run cmd/test/main.go"
					},
				]
			}
		}
	},
	#workflow & {
		_name:    "main-build"
		workflow: github.#Workflow & {
			on: {
				push: {
					branches: [
						"main",
					]
					"tags-ignore": [
						"*",
					]
				}
			}

			jobs: "\(_name)": {
				steps: [
					#checkoutCode,
					#setupGo,
					#pipeline & {
						name: "Build Pipeline"
						run:  "go run cmd/build/main.go"
					},
				]
			}
		}
	},
]

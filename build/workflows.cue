package build

import "json.schemastore.org/github"

#workflow: {
	filename: string
	workflow: github.#Workflow
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
		"go-version-file":       "go.mod"
		"check-latest":          true
		cache:                   true
		"cache-dependency-path": "go.sum"
	}
}

pr: #workflow & {
	filename: "pr.yaml"
	workflow: github.#Workflow & {
		name: "PR"
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

		permissions: "read-all"

		jobs: pr: {
			"runs-on": "ubuntu-latest"
			steps: [
				#checkoutCode,
				#setupGo,
				{
					name:                "Verification Pipeline"
					run:                 "go run cmd/build/test.go"
					"working-directory": "./build"
				},
			]
		}
	}
}

workflows: [pr]

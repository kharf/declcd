<br>
<div align="center">
	<img src="./docs/declcd-light.png#gh-light-mode-only">
	<img src="/docs/declcd.png#gh-dark-mode-only">
  <p align="center">
		<strong>A Declarative Continuous Delivery Toolkit For Kubernetes</strong>
  </p>
  <p>
		<img src="https://img.shields.io/github/actions/workflow/status/kharf/declcd/test.yaml"/>
		<a href="https://goreportcard.com/report/github.com/kharf/declcd"><img src="https://goreportcard.com/badge/github.com/kharf/declcd"/></a>
  </p>
</div>
<br>

## What is GitOps?
GitOps is a way of implementing Continuous Deployment for cloud native applications by having a Git repository that contains declarative descriptions of the desired infrastructure and applications and an automated process to reconcile the production environment with the desired state in the repository.

## Why Declcd?
Traditional GitOps tools often rely on YAML for configuration, which can lead to verbosity and complexity. Declcd leverages [CUE](https://cuelang.org/), a configuration language with a more concise and expressive syntax, making it easier to define and maintain your desired cluster state.

![Overview](./docs/declcd-flow.png)

## Documentation
To learn more about Declcd, visit [declcd.io](https://declcd.io/documentation/overview/)

## Contributions

We welcome contributions! To contribute to Declcd, follow these steps:

1. Fork the repository.
2. Create a new branch for your feature or bug fix.
3. Make your changes.
4. Create tests and run them in a containerized environment via Dagger with:
    ```bash
    cd build/
    # Run all tests
    go run cmd/test/main.go

    # Or run a specific test
    go run cmd/test/main.go MyTest pkg/mypackage
    ```
5. Create a PR.
6. Ensure that your code passes the CI/CD checks.
For more information, see [CONTRIBUTING.md]().

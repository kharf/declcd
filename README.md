# Declcd - A Declarative Continuous Delivery Toolkit For Kubernetes

## Table of Contents
- [Introduction](#introduction)
- [Installation](#installation)
- [Getting Started](#getting-started)
- [Contributions](#contributions)
- [License](#license)

## Introduction

Traditional GitOps tools often rely on YAML for configuration, which can lead to verbosity and complexity. Declcd leverages [CUE](https://cuelang.org/), a configuration language with a more concise and expressive syntax, making it easier to define and maintain your desired cluster state.

## Installation

> [!IMPORTANT]
> Currently we don't maintain our binaries in any package manager.

Linux(x86_64):

```bash
curl -L -o declcd https://github.com/kharf/declcd/releases/download/v0.10.0/declcd-linux-amd64
chmod +x declcd
./declcd -h
```

MacOS(x86_64):

```bash
curl -L -o declcd https://github.com/kharf/declcd/releases/download/v0.10.0/declcd-darwin-amd64
chmod +x declcd
./declcd -h
```

MacOS(arm64):

```bash
curl -L -o declcd https://github.com/kharf/declcd/releases/download/v0.10.0/declcd-darwin-arm64
chmod +x declcd
./declcd -h
```

Windows(x86_64):

```bash
curl -L -o declcd https://github.com/kharf/declcd/releases/download/v0.10.0/declcd-windows-amd64
```

## Getting Started

> [!TIP]
> It is strongly recommended to familiarize yourself with [CUE](https://cuelang.org/) before you begin, as it is one of the cornerstones of Declcd.

### Basics of Declcd

While Declcd does not enforce any kind of repository structure, there is one constraint for the declaration of the cluster state.
Every top-level CUE value in a package, which is not hidden and not a [Definition](https://cuelang.org/docs/tour/basics/definitions/), has to be what Declcd calls a *Component*.
Declcd Components effectively describe the desired cluster state and currently exist in two forms: *Manifests* and *HelmReleases*.
A *Manifest* is a typical [Kubernetes Object](https://kubernetes.io/docs/concepts/overview/working-with-objects/), which you would normally describe in yaml format.
A *HelmRelease* is an instance of a [Helm](https://helm.sh/docs/intro/using_helm/) Chart.
All Components share the attribute to specify Dependencies to other Components. This helps Declcd to identify the correct order in which to apply all objects onto a Kubernetes cluster.

> [!IMPORTANT]
> Dependency relationships are represented in the form of a Directed Acyclic Graph, thus cyclic dependencies lead to errors.


### Initialize a GitOps Repository

```bash
    mkdir mygitops
    cd mygitops
    git init
    # init Declcd gitops repository as a CUE module
    declcd init github.com/user/repo@v0
```
See [CUE module reference](https://cuelang.org/docs/reference/modules/#module-path) for valid CUE module paths.

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

## License

Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.


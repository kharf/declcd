# Declcd - A Declarative Continuous Delivery Toolkit For Kubernetes

## Table of Contents
- [Introduction](#introduction)
- [Getting Started](#getting-started)
- [Usage](#usage)
- [Contributions](#contributions)
- [License](#license)

## Introduction

Traditional GitOps tools often rely on YAML for configuration, which can lead to verbosity and complexity. `Declcd` leverages [CUE](https://cuelang.org/), a configuration language with a more concise and expressive syntax, making it easier to define and maintain your desired cluster state.

## Getting Started

Follow these steps to get started with `Declcd`:

### Installation

Currently we don't maintain our binaries in any package manager.

Linux(x86_64):

```bash
curl -L -o declcd https://github.com/kharf/declcd/releases/download/v0.9.8/declcd-linux-amd64
chmod +x declcd
./declcd -h
```

MacOS(x86_64):

```bash
curl -L -o declcd https://github.com/kharf/declcd/releases/download/v0.9.8/declcd-darwin-amd64
chmod +x declcd
./declcd -h
```

MacOS(arm64):

```bash
curl -L -o declcd https://github.com/kharf/declcd/releases/download/v0.9.8/declcd-darwin-arm64
chmod +x declcd
./declcd -h
```

Windows(x86_64):

```bash
curl -L -o declcd https://github.com/kharf/declcd/releases/download/v0.9.8/declcd-windows-amd64
```

### Initialize a GitOps Repository

```bash
    mkdir mygitops
    cd mygitops
    git init
    # init Declcd gitops repository as a CUE module
    declcd init github.com/user/repo@v0
```
See [CUE module reference](https://cuelang.org/docs/reference/modules/#module-path) for valid CUE module paths.

## Usage
Describe Declcd usage here (Declcd packages and components).

For more detailed instructions and examples, refer to the documentation.

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


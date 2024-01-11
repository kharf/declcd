# declcd - A Declarative Continuous Delivery For Kubernetes

`declcd` is a GitOps toolkit designed for Kubernetes, utilizing the power of Cue instead of YAML for configuration. It allows you to define and maintain the desired state of your Kubernetes cluster in a concise and expressive manner using Cue's declarative syntax.

## Table of Contents
- [Introduction](#introduction)
- [Features](#features)
- [Getting Started](#getting-started)
- [Usage](#usage)
- [Contributions](#contributions)
- [License](#license)

## Introduction

Traditional GitOps tools often rely on YAML for configuration, which can lead to verbosity and complexity. `declcd` leverages Cue, a configuration language with a more concise and expressive syntax, making it easier to define and maintain your desired cluster state.

## Features

- **Declarative Syntax:** Define the desired state of your Kubernetes cluster using Cue's declarative syntax, enhancing readability and maintainability.
- **Kubernetes Integration:** Seamless integration with Kubernetes, allowing you to manage your applications and configurations effortlessly.
- **Scalability:** Handle complex cluster state scenarios with ease, thanks to Cue's expressive and composable nature.
- **Extensibility:** Easily extend and customize your desired cluster state definitions to fit your specific requirements.

## Getting Started

Follow these steps to get started with `declcd`:

1. **Installation:**

2. **Initialize a GitOps Repository:**
```bash
    declcd init mygitops
    cd mygitops
```

3. **Define Desired Cluster State:**
Edit the declcd.cue file to describe the desired state of your Kubernetes cluster using Cue syntax.

4. **Apply Changes:**
```bash
git add declcd.cue
```
For more detailed instructions and examples, refer to the documentation.

## Contributions

We welcome contributions! To contribute to declcd, follow these steps:

1. Fork the repository.
2. Create a new branch for your feature or bug fix.
3. Make your changes and submit a pull request.
4. Ensure that your code passes the CI/CD checks.
For more information, see CONTRIBUTING.md.

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


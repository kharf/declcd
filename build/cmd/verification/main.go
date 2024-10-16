// Copyright 2024 kharf
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"os"

	"github.com/kharf/navecd/build/internal/build"
)

func main() {
	if err := build.RunWith(
		build.WorkflowsGen{Export: true},
		build.CommitWorkflows,
		build.TestAll,
	); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

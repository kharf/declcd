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
	args := os.Args[1:]
	if len(args) < 1 {
		fmt.Println("Version arg required")
		os.Exit(1)
	}
	version := args[0]
	var prevTag string
	if len(args) > 1 {
		prevTag = args[1]
	}

	if err := build.RunWith(
		build.TestAll,
		build.Publish{
			Version:     version,
			PreviousTag: prevTag,
		},
	); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

package main

import (
	"fmt"
	"os"

	"github.com/kharf/declcd/build/internal/build"
)

func main() {
	if err := build.RunWith(
		build.TestAll,
		build.Build,
		build.Publish,
	); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

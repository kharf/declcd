package main

import (
	"fmt"
	"os"

	"github.com/kharf/declcd/build/internal/build"
)

func main() {
	// Only build when tests pass
	if err := build.RunWith(
		build.TestAll,
		build.Build("development"),
	); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

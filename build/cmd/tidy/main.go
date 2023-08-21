package main

import (
	"fmt"
	"os"

	"github.com/kharf/declcd/build/internal/build"
)

func main() {
	if err := build.RunWith(
		build.Tidy,
	); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
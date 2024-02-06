package main

import (
	"fmt"
	"os"

	"github.com/kharf/declcd/build/internal/build"
)

func main() {
	args := os.Args[1:]
	benchToRun := build.BenchAllArg
	if len(args) > 0 {
		benchToRun = args[0]
	}
	var pkgs string
	if len(args) > 1 {
		pkgs = args[1]
	}
	if err := build.RunWith(
		build.Bench{ID: benchToRun, Package: pkgs},
	); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

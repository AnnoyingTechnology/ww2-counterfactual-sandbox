package main

import (
	"context"
	"fmt"
	"os"

	"github.com/AnnoyingTechnology/ww2-counterfactual-sandbox/internal/cli"
)

func main() {
	if err := cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

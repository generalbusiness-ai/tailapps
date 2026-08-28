package main

import (
	"context"
	"fmt"
	"os"

	"github.com/generalbusiness-ai/tailapp/internal/cli"
)

func main() {
	if err := cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "tailapp:", err)
		os.Exit(1)
	}
}

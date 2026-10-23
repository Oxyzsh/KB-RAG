package main

import (
	"context"
	"os"
	"os/signal"

	"kb-rag/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

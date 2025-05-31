package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/IljaN/opencloud-sftp/pkg/command"
	"github.com/IljaN/opencloud-sftp/pkg/config/defaults"
)

func main() {
	cfg := defaults.DefaultConfig()
	cfg.Context, _ = signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP)
	if err := command.Execute(cfg); err != nil {
		os.Exit(1)
	}
}

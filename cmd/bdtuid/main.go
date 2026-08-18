package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"bdtui/internal/daemon"
	"bdtui/internal/orch"
)

func main() {
	socketPath := flag.String("socket", daemon.DefaultSocketPath(), "Unix domain socket path")
	dbPath := flag.String("db", daemon.DefaultDBPath(), "SQLite database path")
	pidPath := flag.String("pidfile", "", "Path to write the daemon PID (defaults to <socket>.pid)")
	flag.Parse()

	if err := run(*socketPath, *dbPath, *pidPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(socketPath, dbPath, pidPath string) error {
	if pidPath == "" {
		pidPath = socketPath + ".pid"
	}
	lockPath := daemon.LockPath(socketPath)

	if err := daemon.EnsureStateDirs(socketPath, dbPath, pidPath, lockPath); err != nil {
		return fmt.Errorf("create state dirs: %w", err)
	}

	lock, err := daemon.AcquireLock(lockPath)
	if err != nil {
		return err
	}
	defer daemon.ReleaseLock(lock)

	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return fmt.Errorf("write pidfile: %w", err)
	}
	defer os.Remove(pidPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	store, err := orch.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	srv := daemon.NewServer(store, socketPath)
	return srv.Serve(ctx)
}

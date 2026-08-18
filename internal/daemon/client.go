package daemon

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"bdtui/internal/daemon/daemonpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps a gRPC connection and the generated Orchestrator client.
type Client struct {
	conn *grpc.ClientConn
	daemonpb.OrchestratorClient
}

// Close releases the underlying connection.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// Dial connects to a running daemon at socketPath. It does not auto-start.
func Dial(socketPath string) (*Client, error) {
	conn, err := grpc.NewClient(
		"passthrough:///bdtuid",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, "unix", socketPath)
		}),
	)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn:              conn,
		OrchestratorClient: daemonpb.NewOrchestratorClient(conn),
	}, nil
}

// Options configures client auto-start behavior.
type Options struct {
	SocketPath string
	DBPath     string
	// Binary is the daemon executable to spawn when no daemon is running.
	// Empty defaults to "bdtuid".
	Binary string
	// StartTimeout bounds how long EnsureDaemon waits for the socket after
	// spawning the daemon.
	StartTimeout time.Duration
}

func (o Options) withDefaults() Options {
	if o.SocketPath == "" {
		o.SocketPath = DefaultSocketPath()
	}
	if o.DBPath == "" {
		o.DBPath = DefaultDBPath()
	}
	if o.Binary == "" {
		o.Binary = "bdtuid"
	}
	if o.StartTimeout == 0 {
		o.StartTimeout = 5 * time.Second
	}
	return o
}

// EnsureDaemon returns a client for a running daemon, spawning one first if no
// live socket is present. The daemon is detached so it survives the client.
func EnsureDaemon(ctx context.Context, opts Options) (*Client, error) {
	opts = opts.withDefaults()

	if socketAlive(ctx, opts.SocketPath) {
		return Dial(opts.SocketPath)
	}
	if err := startDaemon(ctx, opts); err != nil {
		return nil, err
	}
	return Dial(opts.SocketPath)
}

func startDaemon(ctx context.Context, opts Options) error {
	logPath := filepath.Join(StateDir(), "bdtuid.log")
	var out io.Writer = io.Discard
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		out = f
		defer f.Close()
	}

	cmd := exec.Command(opts.Binary, "--socket", opts.SocketPath, "--db", opts.DBPath)
	cmd.Stdin = nil
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon %q: %w", opts.Binary, err)
	}
	// Detach so the daemon is not reaped with the client.
	if err := cmd.Process.Release(); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("release daemon process: %w", err)
	}

	if err := waitForSocket(ctx, opts.SocketPath, opts.StartTimeout); err != nil {
		return fmt.Errorf("daemon did not become ready: %w", err)
	}
	return nil
}

func socketAlive(ctx context.Context, socketPath string) bool {
	d := net.Dialer{Timeout: 200 * time.Millisecond}
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func waitForSocket(ctx context.Context, socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if socketAlive(ctx, socketPath) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for socket %s", socketPath)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

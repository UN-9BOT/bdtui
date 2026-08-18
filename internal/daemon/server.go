package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"bdtui/internal/daemon/daemonpb"
	"bdtui/internal/orch"

	"google.golang.org/grpc"
)

// gracefulStopTimeout bounds GracefulStop so an open server stream (e.g.
// StreamEvents) cannot block daemon shutdown forever.
const gracefulStopTimeout = 5 * time.Second

// Server owns the gRPC listener and service lifecycle for the daemon.
type Server struct {
	grpcServer *grpc.Server
	store      *orch.Store
	socketPath string
	listener   net.Listener
}

// NewServer builds a server that serves the Orchestrator API over the given
// Unix domain socket path.
func NewServer(store *orch.Store, socketPath string) *Server {
	s := &Server{
		grpcServer: grpc.NewServer(),
		store:      store,
		socketPath: socketPath,
	}
	daemonpb.RegisterOrchestratorServer(s.grpcServer, NewService(store))
	return s
}

// SocketPath returns the path this server is bound to.
func (s *Server) SocketPath() string {
	return s.socketPath
}

// Serve binds the Unix socket and serves until ctx is cancelled. A pre-existing
// socket file is removed only after proving it is stale (no live listener);
// a live socket returns an error instead of being clobbered.
func (s *Server) Serve(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0o700); err != nil {
		return err
	}
	if socketInUse(s.socketPath) {
		return fmt.Errorf("daemon socket %s is already in use", s.socketPath)
	}
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return err
	}
	s.listener = ln

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.grpcServer.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		s.shutdownGracefully()
		_ = os.Remove(s.socketPath)
		return nil
	case err := <-errCh:
		return err
	}
}

// shutdownGracefully stops the server, but only waits for in-flight RPCs up to
// gracefulStopTimeout. Open streaming handlers that never return are forcibly
// terminated after the timeout.
func (s *Server) shutdownGracefully() {
	done := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(gracefulStopTimeout):
		s.grpcServer.Stop()
	}
}

// Stop immediately terminates the gRPC server and removes the socket file.
// It is primarily for tests and error paths; Serve handles graceful shutdown.
func (s *Server) Stop() {
	if s.grpcServer != nil {
		s.grpcServer.Stop()
	}
	_ = os.Remove(s.socketPath)
}

func socketInUse(path string) bool {
	conn, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

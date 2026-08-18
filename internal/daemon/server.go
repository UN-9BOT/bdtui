package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"

	"bdtui/internal/daemon/daemonpb"
	"bdtui/internal/orch"

	"google.golang.org/grpc"
)

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

// Serve binds the Unix socket, removing any stale socket file first, and
// serves until ctx is cancelled. On cancellation it performs a graceful stop.
func (s *Server) Serve(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0o700); err != nil {
		return err
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
		s.grpcServer.GracefulStop()
		_ = os.Remove(s.socketPath)
		return nil
	case err := <-errCh:
		return err
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

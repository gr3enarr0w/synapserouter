package mcpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
)

// Server exposes a tool registry as an MCP server over HTTP.
type Server struct {
	registry *tools.Registry
	workDir  string
	handler  *Handler
	token    string // bearer token for auth (empty = no auth)
}

// NewServer creates an MCP server backed by the given tool registry.
// Defaults to auto_approve permission mode. Use SetPermissions to change.
func NewServer(registry *tools.Registry, workDir string) *Server {
	pc := tools.NewPermissionChecker(tools.ModeAutoApprove)
	s := &Server{
		registry: registry,
		workDir:  workDir,
	}
	s.handler = &Handler{
		registry:    registry,
		permissions: pc,
		workDir:     workDir,
	}
	return s
}

// SetPermissions sets the permission mode for tool execution.
func (s *Server) SetPermissions(pc *tools.PermissionChecker) {
	s.handler.permissions = pc
}

// SetToken sets a bearer token for authentication. If empty, no auth is enforced.
func (s *Server) SetToken(token string) {
	s.token = token
}

// Token returns the current auth token.
func (s *Server) Token() string {
	return s.token
}

// Routes returns an http.Handler that handles MCP protocol requests.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp/tools/list", s.handler.HandleToolsList)
	mux.HandleFunc("/mcp/tools/call", s.handler.HandleToolsCall)
	mux.HandleFunc("/mcp/initialize", s.handler.HandleInitialize)

	if s.token != "" {
		return AuthMiddleware(s.token, mux)
	}
	return mux
}

// Handler returns the HTTP handler for registering on an external mux.
func (s *Server) Handler() *Handler {
	return s.handler
}

// Serve starts a standalone MCP HTTP server.
func (s *Server) Serve(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: s.Routes(),
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	return srv.ListenAndServe()
}

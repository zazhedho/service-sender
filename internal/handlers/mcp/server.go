package mcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	interfaceemail "service-sender/internal/interfaces/email"
	"service-sender/pkg/config"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type requestContextKey struct{}

func NewHTTPHandler(cfg config.MCPConfig, services map[string]interfaceemail.EmailSender, appName string) http.Handler {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "service-sender", Version: "1.0.0"}, nil)
	registerEmailTools(server, cfg, services, appName)
	transport := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return server
	}, &mcpsdk.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 1 << 20,
		PropagateRequestCancellation: true,
	})
	protected := http.NewCrossOriginProtection().Handler(transport)
	wantKey := sha256.Sum256([]byte(cfg.ServerKey))
	mux := http.NewServeMux()
	mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		authorization := req.Header.Values("Authorization")
		if len(authorization) != 1 {
			unauthorized(w)
			return
		}
		parts := strings.Fields(authorization[0])
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || cfg.ServerKey == "" {
			unauthorized(w)
			return
		}
		gotKey := sha256.Sum256([]byte(parts[1]))
		if subtle.ConstantTimeCompare(gotKey[:], wantKey[:]) != 1 {
			unauthorized(w)
			return
		}
		ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
		defer cancel()
		// Legacy MCP sessions detach cancellation but preserve context values.
		ctx = context.WithValue(ctx, requestContextKey{}, ctx)
		protected.ServeHTTP(w, req.WithContext(ctx))
	}))
	return mux
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

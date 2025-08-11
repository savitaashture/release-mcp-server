package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tektoncd/release-mcp/internal/tools"
	"go.etcd.io/etcd/version"
	"k8s.io/client-go/tools/clientcmd"
	filteredinformerfactory "knative.dev/pkg/client/injection/kube/informers/factory/filtered"
	"knative.dev/pkg/injection"
	"knative.dev/pkg/signals"
)

// ManagedByLabelKey is the label key used to mark what is managing this resource
const ManagedByLabelKey = "app.kubernetes.io/managed-by"

func main() {
	// Configure logging
	logHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	// Parse command line flags
	var transport string
	var httpAddr string
	flag.StringVar(&transport, "transport", "http", "Transport type (stdio or http)")
	flag.StringVar(&httpAddr, "address", ":3000", "Address to bind the HTTP server to")
	flag.Parse()

	if httpAddr == "" && transport == "http" {
		slog.Error("-address is required when transport is set to 'http'")
		os.Exit(1)
	}

	// Create MCP server
	impl := &mcp.Implementation{
		Name:    "Tekton Release MCP Server",
		Version: version.Version,
		Title:   "Tekton Release Management Server",
	}
	opts := &mcp.ServerOptions{
		Instructions: `This server provides tools for managing Tekton releases:
- Creating release branches
- Configuring hack repository
- Managing ReleasePlanAdmission and ReleasePlan files`,
	}
	s := mcp.NewServer(impl, opts)

	// Create context with cancellation
	ctx := signals.NewContext()

	// Load kubernetes configuration
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	cfg, err := kubeConfig.ClientConfig()
	if err != nil {
		slog.Error("Failed to get Kubernetes config", "error", err)
		os.Exit(1)
	}

	// Configure and start informers
	ctx = filteredinformerfactory.WithSelectors(ctx, ManagedByLabelKey)
	ctx, startInformers := injection.EnableInjectionOrDie(ctx, cfg)
	startInformers()

	// Add tools to the server
	if err = tools.Add(ctx, s); err != nil {
		slog.Error("Failed to add tools", "error", err)
		os.Exit(1)
	}

	slog.Info("Starting the server")

	errC := make(chan error, 1)

	switch transport {
	case "http":
		// Configure HTTP server with timeouts and handlers
		streamableHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server { return s }, nil)
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			streamableHandler.ServeHTTP(w, r.WithContext(ctx))
		})

		server := &http.Server{
			Addr:              httpAddr,
			Handler:           handler,
			ReadHeaderTimeout: 3 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
		}

		go func() {
			slog.Info("Server listening", "address", httpAddr)
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errC <- fmt.Errorf("server error: %w", err)
			}
		}()

	case "stdio":
		go func() {
			if err := s.Run(ctx, mcp.NewStdioTransport()); err != nil {
				errC <- fmt.Errorf("stdio transport error: %w", err)
			}
		}()
		slog.Info("Server running on stdio")

	default:
		slog.Error("Invalid transport type", "transport", transport)
		os.Exit(1)
	}

	// Wait for shutdown signal or error
	select {
	case <-ctx.Done():
		slog.Info("Received shutdown signal")
	case err := <-errC:
		slog.Error("Server error", "error", err)
		os.Exit(1)
	}

	slog.Info("Server shutting down")
}

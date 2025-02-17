package metrics

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"k8s.io/client-go/rest"
)

// ServerRunnable is a struct that implements Runnable interface.
type ServerRunnable struct {
	httpServer       *http.Server
	restConfig       *rest.Config
	namespaceMetrics *NamespaceMetrics

	kubeApiserver string
	nodeNameOrIP  string
	nodePort      string
	nodePath      string
}

// ServerRunnableOpts is a struct that contains options for ServerRunnable.
type ServerRunnableOpts struct {
	RestConfig *rest.Config

	KubeApiserver string
	NodeNameOrIP  string
	NodePort      string
	NodePath      string
}

// NewServerRunnable is a constructor that creates http.Server and handler.
func NewServerRunnable(
	restConfig *rest.Config,
	port string,
	nm *NamespaceMetrics,
	kubeApiserver, nodeNameOrIP, nodePort string,
) *ServerRunnable {
	mux := http.NewServeMux()
	nodePath := "/"
	if kubeApiserver != "" {
		nodePath = fmt.Sprintf("/api/v1/nodes/%s/proxy/", nodeNameOrIP)
	}

	sharedHandlerMetrics := Handler(nm, &ServerRunnableOpts{
		KubeApiserver: kubeApiserver,
		RestConfig:    restConfig,
		NodeNameOrIP:  nodeNameOrIP,
		NodePort:      nodePort,
		NodePath:      fmt.Sprintf("%smetrics", nodePath),
	})

	sharedHandlerCadvisorMetrics := Handler(nm, &ServerRunnableOpts{
		KubeApiserver: kubeApiserver,
		RestConfig:    restConfig,
		NodeNameOrIP:  nodeNameOrIP,
		NodePort:      nodePort,
		NodePath:      fmt.Sprintf("%smetrics/cadvisor", nodePath),
	})

	mux.Handle("/metrics", sharedHandlerMetrics)
	mux.Handle("/metrics/cadvisor", sharedHandlerCadvisorMetrics)

	return &ServerRunnable{
		restConfig: restConfig,
		httpServer: &http.Server{
			Addr:    ":" + port,
			Handler: mux,
		},
		namespaceMetrics: nm,
		kubeApiserver:    kubeApiserver,
		nodeNameOrIP:     nodeNameOrIP,
		nodePort:         nodePort,
	}
}

// Start will be called automatically when mgr.Start(...).
func (sr *ServerRunnable) Start(ctx context.Context) error {
	log.Printf("Starting custom metrics server on %s\n", sr.httpServer.Addr)

	// Start server in a separate goroutine to not block Start().
	go func() {
		if err := sr.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v\n", err)
		}
	}()

	// Wait until context is done.
	<-ctx.Done()

	log.Printf("Shutting down metrics server on %s...\n", sr.httpServer.Addr)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return sr.httpServer.Shutdown(shutdownCtx)
}

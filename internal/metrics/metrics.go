package metrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/log"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"google.golang.org/protobuf/proto"
	"k8s.io/client-go/rest"
)

// NamespaceMetrics stores namespace names and their labels.
type NamespaceMetrics struct {
	Namespaces map[string]map[string]string
}

// NewNamespaceMetrics creates a new NamespaceMetrics instance.
func NewNamespaceMetrics() *NamespaceMetrics {
	return &NamespaceMetrics{
		Namespaces: make(map[string]map[string]string),
	}
}

// Handler handles HTTP requests for Prometheus metrics.
func Handler(nm *NamespaceMetrics, opts *ServerRunnableOpts) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logger := log.FromContext(ctx).WithName("metrics.Handler")
		logger.V(1).Info("serving metrics", "path", r.URL.Path)
		data, err := FetchAndProcessMetrics(ctx, nm, opts)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to fetch/process metrics: %v", err),
				http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write(data)
	})
}

// FetchAndProcessMetrics fetches metrics from kubelet and returns enhanced metrics.
func FetchAndProcessMetrics(
	ctx context.Context,
	nm *NamespaceMetrics,
	opts *ServerRunnableOpts,
) ([]byte, error) {
	logger := log.FromContext(ctx).WithName("metrics.FetchAndProcessMetrics")
	logger.V(1).Info("fetching metrics")
	var raw []byte
	var err error

	raw, err = fetchMetrics(
		// TODO: Fix insecureSkipVerify
		ctx, opts.RestConfig, opts, opts.RestConfig.Insecure,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch error: %w", err)
	}

	var parser expfmt.TextParser
	metricFamilies, err := parser.TextToMetricFamilies(strings.NewReader(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse metrics: %w", err)
	}

	logger.V(1).Info("enriching metrics")

	enriched, err := EnrichMetricFamilies(metricFamilies, nm)
	if err != nil {
		return nil, fmt.Errorf("failed to enrich metrics: %w", err)
	}

	return []byte(enriched), nil
}

// fetchDirectFromKubelet call to nodeIP:nodePort/nodePath.
func fetchMetrics(
	ctx context.Context, cfg *rest.Config, otps *ServerRunnableOpts, insecureSkipVerify bool,
) ([]byte, error) {
	logger := log.FromContext(ctx)
	nodeIP := otps.NodeNameOrIP
	if otps.KubeApiserver != "" {
		nodeIP = otps.KubeApiserver
	}

	url := fmt.Sprintf("https://%s:%s%s", nodeIP, otps.NodePort, otps.NodePath)
	logger.V(1).Info("fetching metrics from", "url", url)

	transport, err := rest.TransportFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport from rest.Config: %w", err)
	}

	if insecureSkipVerify {
		if httpTransport, ok := transport.(*http.Transport); ok {
			httpTransport.TLSClientConfig.InsecureSkipVerify = true
		}
	}

	httpClient := &http.Client{Transport: transport}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bad status code: %d, body: %s", resp.StatusCode, string(b))
	}

	return io.ReadAll(resp.Body)
}

// EnrichMetricFamilies enriches metrics with extra labels.
func EnrichMetricFamilies(metricFamilies map[string]*dto.MetricFamily, nm *NamespaceMetrics) (string, error) {
	for _, mf := range metricFamilies {
		for _, metric := range mf.Metric {
			var nsValue string

			for _, lbl := range metric.Label {
				if lbl.GetName() == "namespace" {
					nsValue = lbl.GetValue()
					break
				}
			}

			if nsValue != "" {
				if extraLabels, ok := nm.Namespaces[nsValue]; ok {
					for k, v := range extraLabels {
						if hasLabel(metric.Label, k) {
							continue
						}
						newLabel := &dto.LabelPair{
							Name:  proto.String(k),
							Value: proto.String(v),
						}
						metric.Label = append(metric.Label, newLabel)
					}
				}
			}
		}
	}

	var sb strings.Builder
	encoder := expfmt.NewEncoder(&sb, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range metricFamilies {
		if err := encoder.Encode(mf); err != nil {
			return "", fmt.Errorf("failed to encode metric family %q: %w", mf.GetName(), err)
		}
	}

	return sb.String(), nil
}

func hasLabel(labels []*dto.LabelPair, name string) bool {
	for _, lbl := range labels {
		if lbl.GetName() == name {
			return true
		}
	}
	return false
}

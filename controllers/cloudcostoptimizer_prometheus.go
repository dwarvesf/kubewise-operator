package controllers

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	optimizationv1alpha1 "github.com/dwarvesf/cloud-cost-optimizer/api/v1alpha1"
)

// setupPrometheusClient initializes the Prometheus client using the provided configuration
func (r *CloudCostOptimizerReconciler) setupPrometheusClient(config optimizationv1alpha1.PrometheusConfig) (v1.API, error) {
	// Validate and potentially modify the server address
	serverAddress, err := validatePrometheusAddress(config.ServerAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid Prometheus server address: %v", err)
	}

	client, err := api.NewClient(api.Config{
		Address: serverAddress,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating Prometheus client: %v", err)
	}
	return v1.NewAPI(client), nil
}

// validatePrometheusAddress ensures the Prometheus server address is valid and has a scheme
func validatePrometheusAddress(address string) (string, error) {
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
		address = "http://" + address
	}

	u, err := url.Parse(address)
	if err != nil {
		return "", fmt.Errorf("failed to parse address: %v", err)
	}

	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid address format, must be in the form of 'http(s)://hostname:port'")
	}

	return u.String(), nil
}

// getHistoricalMetric retrieves the average metric value for the specified duration
func (r *CloudCostOptimizerReconciler) getHistoricalMetric(ctx context.Context, pod *corev1.Pod, metric string, prometheusClient v1.API, duration time.Duration) (float64, error) {
	query := fmt.Sprintf("avg_over_time(%s{pod=\"%s\",namespace=\"%s\"}[%s])", metric, pod.Name, pod.Namespace, duration.String())
	result, warnings, err := prometheusClient.Query(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("prometheus query failed: %v", err)
	}
	if len(warnings) > 0 {
		log.FromContext(ctx).Info("Prometheus query returned warnings", "warnings", warnings)
	}

	if vector, ok := result.(model.Vector); ok && len(vector) > 0 {
		return float64(vector[0].Value), nil
	}

	return 0, fmt.Errorf("no data found for metric %s", metric)
}

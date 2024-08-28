package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	optimizationv1alpha1 "github.com/dwarvesf/cloud-cost-optimizer/api/v1alpha1"
	"github.com/dwarvesf/cloud-cost-optimizer/internal/discord"
)

// CloudCostOptimizerReconciler reconciles a CloudCostOptimizer object
type CloudCostOptimizerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// ResourceRecommendation represents a recommendation for resource allocation
type ResourceRecommendation struct {
	PodName               string
	Namespace             string
	ContainerName         string
	UsageCPU              *resource.Quantity
	CurrentRequestsCPU    *resource.Quantity
	RecommendedCPU        *resource.Quantity
	UsageMemory           *resource.Quantity
	CurrentRequestsMemory *resource.Quantity
	RecommendedMemory     *resource.Quantity
}

//+kubebuilder:rbac:groups=optimization.dwarvesf.com,resources=cloudcostoptimizers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=optimization.dwarvesf.com,resources=cloudcostoptimizers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=optimization.dwarvesf.com,resources=cloudcostoptimizers/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *CloudCostOptimizerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("Reconciling CloudCostOptimizer")

	// Fetch the CloudCostOptimizer instance
	cloudCostOptimizer := &optimizationv1alpha1.CloudCostOptimizer{}
	err := r.Get(ctx, req.NamespacedName, cloudCostOptimizer)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check the last reconcile time to avoid frequent requeuing
	now := metav1.Now()
	if cloudCostOptimizer.Status.LastReconcileTime != nil && now.Sub(cloudCostOptimizer.Status.LastReconcileTime.Time) < cloudCostOptimizer.Spec.AnalysisInterval.Duration {
		logger.Info("Reconcile skipped as it was triggered too soon after the last run")
		return ctrl.Result{RequeueAfter: cloudCostOptimizer.Spec.AnalysisInterval.Duration - now.Sub(cloudCostOptimizer.Status.LastReconcileTime.Time)}, nil
	}

	// Update the last reconcile time to the current time
	cloudCostOptimizer.Status.LastReconcileTime = &now

	// Initialize Prometheus client
	prometheusClient, err := r.setupPrometheusClient(cloudCostOptimizer.Spec.PrometheusConfig)
	if err != nil {
		logger.Error(err, "Failed to setup Prometheus client")
		return ctrl.Result{}, err
	}

	// Initialize Discord service
	discordService := discord.NewDiscordService(cloudCostOptimizer.Spec.DiscordConfig.WebhookURL)

	allRecommendations := []ResourceRecommendation{}

	// Iterate through each target
	for _, target := range cloudCostOptimizer.Spec.Targets {
		targetRecommendations, err := r.analyzeTarget(ctx, target, prometheusClient, cloudCostOptimizer.Spec.PrometheusConfig.HistoricalMetricDuration.Duration)
		if err != nil {
			logger.Error(err, "Failed to analyze target", "target", target)
			continue
		}
		allRecommendations = append(allRecommendations, targetRecommendations...)
	}

	// Sort recommendations to ensure consistent ordering
	sort.SliceStable(allRecommendations, func(i, j int) bool {
		return allRecommendations[i].Namespace+allRecommendations[i].PodName < allRecommendations[j].Namespace+allRecommendations[j].PodName
	})

	// Create a hash of the recommendations
	newRecommendationsHash := hashRecommendations(allRecommendations)

	// Compare the new hash with the stored hash
	if newRecommendationsHash != cloudCostOptimizer.Status.RecommendationsHash {
		// If there are changes, send recommendations to Discord
		if len(allRecommendations) > 0 {
			message := formatRecommendationsMessage(allRecommendations)
			if err := discordService.SendMessage(message); err != nil {
				logger.Error(err, "Failed to send Discord message", "message_length", len(message))
			}
		} else {
			logger.Info("No resource optimization recommendations found")
		}

		// Update the status of the CloudCostOptimizer resource with new recommendations and hash
		cloudCostOptimizer.Status.Recommendations = formatRecommendationsStatus(allRecommendations)
		cloudCostOptimizer.Status.RecommendationsHash = newRecommendationsHash
		if err := r.Status().Update(ctx, cloudCostOptimizer); err != nil {
			logger.Error(err, "Failed to update CloudCostOptimizer status")
			return ctrl.Result{}, err
		}
	} else {
		logger.Info("No changes in recommendations; skipping Discord notification")
	}

	return ctrl.Result{RequeueAfter: cloudCostOptimizer.Spec.AnalysisInterval.Duration}, nil
}

func (r *CloudCostOptimizerReconciler) analyzeTarget(ctx context.Context, target optimizationv1alpha1.Target, prometheusClient v1.API, duration time.Duration) ([]ResourceRecommendation, error) {
	logger := log.FromContext(ctx)
	var recommendations []ResourceRecommendation

	// List all pods in the target namespaces
	var podList corev1.PodList
	listOpts := []client.ListOption{}

	// Check if the namespace list contains "*", which means all namespaces
	applyToAllNamespaces := false
	for _, ns := range target.Namespaces {
		if ns == "*" {
			applyToAllNamespaces = true
			break
		}
	}

	if applyToAllNamespaces {
		// If "*" is present, list all pods in all namespaces
		logger.Info("Applying to all namespaces")
		if err := r.List(ctx, &podList, listOpts...); err != nil {
			logger.Error(err, "Failed to list pods in all namespaces")
			return nil, err
		}
	} else if len(target.Namespaces) > 0 {
		// Compile regex patterns for namespace matching
		var regexPatterns []*regexp.Regexp
		for _, ns := range target.Namespaces {
			pattern, err := regexp.Compile(ns)
			if err != nil {
				logger.Error(err, "Invalid regex pattern", "pattern", ns)
				continue
			}
			regexPatterns = append(regexPatterns, pattern)
		}

		// List all namespaces
		var namespaceList corev1.NamespaceList
		if err := r.List(ctx, &namespaceList); err != nil {
			logger.Error(err, "Failed to list namespaces")
			return nil, err
		}

		// Filter namespaces based on regex patterns
		var matchedNamespaces []string
		for _, ns := range namespaceList.Items {
			for _, pattern := range regexPatterns {
				if pattern.MatchString(ns.Name) {
					matchedNamespaces = append(matchedNamespaces, ns.Name)
					break
				}
			}
		}

		// List pods for each matched namespace
		var allPods []corev1.Pod
		for _, ns := range matchedNamespaces {
			logger.Info("Listing pods in namespace", "namespace", ns)
			namespaceOpts := append(listOpts, client.InNamespace(ns))
			var namespacePodList corev1.PodList
			if err := r.List(ctx, &namespacePodList, namespaceOpts...); err != nil {
				logger.Error(err, "Failed to list pods in namespace", "namespace", ns)
				return nil, err
			}
			allPods = append(allPods, namespacePodList.Items...)
		}
		podList.Items = allPods
	} else {
		// If no namespaces specified, but "*" isn't present, list all pods
		logger.Info("Listing all pods in the cluster")
		if err := r.List(ctx, &podList, listOpts...); err != nil {
			logger.Error(err, "Failed to list pods")
			return nil, err
		}
	}

	// Analyze each pod for potential waste and generate recommendations
	for _, pod := range podList.Items {
		logger.Info("Analyzing pod", "pod", pod.Name)
		podRecommendations, err := r.analyzePodResources(ctx, &pod, prometheusClient, duration)
		if err != nil {
			logger.Error(err, "Failed to analyze pod", "pod", pod.Name)
			continue
		}
		recommendations = append(recommendations, podRecommendations...)

		// Check if automateOptimization is enabled for this target
		if target.AutomateOptimization {
			if err := r.applyOptimization(ctx, &pod, podRecommendations); err != nil {
				logger.Error(err, "Failed to apply optimization", "pod", pod.Name)
			}
		}
	}

	return recommendations, nil
}

// applyOptimization applies the recommended resource changes to the pod
func (r *CloudCostOptimizerReconciler) applyOptimization(ctx context.Context, pod *corev1.Pod, recommendations []ResourceRecommendation) error {
	logger := log.FromContext(ctx)
	logger.Info("Applying optimization", "pod", pod.Name)

	for _, rec := range recommendations {
		for i, container := range pod.Spec.Containers {
			if container.Name == rec.ContainerName {
				pod.Spec.Containers[i].Resources.Requests[corev1.ResourceCPU] = *rec.RecommendedCPU
				pod.Spec.Containers[i].Resources.Requests[corev1.ResourceMemory] = *rec.RecommendedMemory
				logger.Info("Updated container resources", "container", container.Name, "cpu", rec.RecommendedCPU, "memory", rec.RecommendedMemory)
			}
		}
	}

	if err := r.Update(ctx, pod); err != nil {
		return fmt.Errorf("failed to update pod: %v", err)
	}

	return nil
}

// hashRecommendations creates a hash of the recommendations
func hashRecommendations(recommendations []ResourceRecommendation) string {
	hash := sha256.New()
	for _, rec := range recommendations {
		hashString := fmt.Sprintf("%s:%s",
			rec.Namespace,
			rec.PodName,
		)
		hash.Write([]byte(hashString))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

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

func (r *CloudCostOptimizerReconciler) analyzePodResources(ctx context.Context, pod *corev1.Pod, prometheusClient v1.API, duration time.Duration) ([]ResourceRecommendation, error) {
	logger := log.FromContext(ctx)

	if isJobPod, jobType := isPodCreatedByJobOrCronJob(pod); isJobPod {
		logger.Info("Skipping job pod", "pod", pod.Name, "jobType", jobType)
		return nil, nil
	}

	var recommendations []ResourceRecommendation

	logger.Info("Analyzing pod", "pod", pod.Name, "namespace", pod.Namespace)

	for _, container := range pod.Spec.Containers {
		logger.Info("Analyzing container", "container", container.Name)

		const (
			minCPU    = 50               // minimum CPU recommendation in milli-cores (50m)
			minMemory = 64 * 1024 * 1024 // minimum Memory recommendation in bytes (64Mi)
		)

		cpuUsage, err := r.getHistoricalMetric(ctx, pod, "container_cpu_usage_seconds_total", prometheusClient, duration)
		if err != nil {
			logger.Error(err, "Failed to get CPU usage", "container", container.Name)
			return nil, fmt.Errorf("failed to get CPU usage: %v", err)
		}

		memoryUsage, err := r.getHistoricalMetric(ctx, pod, "container_memory_usage_bytes", prometheusClient, duration)
		if err != nil {
			logger.Error(err, "Failed to get memory usage", "container", container.Name)
			return nil, fmt.Errorf("failed to get memory usage: %v", err)
		}

		cpuRequest := container.Resources.Requests.Cpu()
		memoryRequest := container.Resources.Requests.Memory()

		logger.Info("Resource usage",
			"container", container.Name,
			"cpuUsage", cpuUsage,
			"cpuRequest", cpuRequest.MilliValue(),
			"memoryUsage", memoryUsage,
			"memoryRequest", memoryRequest.Value())

		recCPU := float64(cpuUsage/1000) * 3
		recMemory := memoryUsage * 3

		if cpuUsage < float64(cpuRequest.MilliValue())*0.5 || memoryUsage < float64(memoryRequest.Value())*0.5 {
			// Calculate reduced resource requests based on the current usage
			if int(recCPU) < minCPU {
				recCPU = minCPU
			} else if recCPU > float64(cpuRequest.MilliValue()) {
				recCPU = float64(cpuUsage/1000) * 2
				if recCPU > float64(cpuRequest.MilliValue()) {
					recCPU = float64(cpuRequest.MilliValue())
				}
			}

			if int(recMemory) < minMemory {
				recMemory = minMemory
			} else if recMemory > float64(memoryRequest.Value()) {
				recMemory = memoryUsage * 2
				if recMemory > float64(memoryRequest.Value()) {
					recMemory = float64(memoryRequest.Value())
				}
			}
			recommendedCPU := resource.NewMilliQuantity(int64(recCPU), resource.DecimalSI)
			recommendedMemory := resource.NewQuantity(int64(recMemory), resource.BinarySI)

			logger.Info("Generating recommendation",
				"container", container.Name,
				"currentCPU", cpuRequest.String(),
				"recommendedCPU", recommendedCPU.String(),
				"currentMemory", memoryRequest.String(),
				"recommendedMemory", recommendedMemory.String())

			recommendations = append(recommendations, ResourceRecommendation{
				PodName:               pod.Name,
				Namespace:             pod.Namespace,
				ContainerName:         container.Name,
				UsageCPU:              resource.NewMilliQuantity(int64(cpuUsage)/1000, resource.DecimalSI),
				CurrentRequestsCPU:    cpuRequest,
				RecommendedCPU:        recommendedCPU,
				UsageMemory:           resource.NewQuantity(int64(memoryUsage), resource.BinarySI),
				CurrentRequestsMemory: memoryRequest,
				RecommendedMemory:     recommendedMemory,
			})
		} else {
			logger.Info("No recommendation needed", "container", container.Name)
		}
	}

	logger.Info("Finished analyzing pod", "pod", pod.Name, "recommendationsCount", len(recommendations))
	return recommendations, nil
}

// formatRecommendationsMessage formats the recommendations for a Discord message
func formatRecommendationsMessage(recommendations []ResourceRecommendation) string {
	var sb strings.Builder
	sb.WriteString("**Resource Optimization Recommendations**\n\n")

	// Group recommendations by namespace and pod
	groupedRecs := make(map[string]map[string][]ResourceRecommendation)
	for _, rec := range recommendations {
		if _, ok := groupedRecs[rec.Namespace]; !ok {
			groupedRecs[rec.Namespace] = make(map[string][]ResourceRecommendation)
		}
		groupedRecs[rec.Namespace][rec.PodName] = append(groupedRecs[rec.Namespace][rec.PodName], rec)
	}

	for namespace, pods := range groupedRecs {
		sb.WriteString(fmt.Sprintf("**Namespace:** %s\n", namespace))
		for podName, recs := range pods {
			sb.WriteString(fmt.Sprintf("Pod: %s\n", podName))
			for _, rec := range recs {
				sb.WriteString(fmt.Sprintf("  [%s] CPU: %s -> %s (Usage: %s) | Mem: %s -> %s (Usage: %s)\n",
					rec.ContainerName,
					formatResourceValue(rec.CurrentRequestsCPU),
					formatResourceValue(rec.RecommendedCPU),
					formatResourceValue(rec.UsageCPU),
					formatResourceValue(rec.CurrentRequestsMemory),
					formatResourceValue(rec.RecommendedMemory),
					formatResourceValue(rec.UsageMemory)))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatResourceValue formats a resource.Quantity value to a human-readable string
func formatResourceValue(quantity *resource.Quantity) string {
	switch quantity.Format {
	case resource.DecimalSI:
		return fmt.Sprintf("%dm", quantity.MilliValue())
	case resource.BinarySI:
		return fmt.Sprintf("%dMi", quantity.Value()/(1024*1024)) // Convert bytes to MiB
	default:
		return quantity.String() // Default to the standard string representation
	}
}

// isPodCreatedByJobOrCronJob checks if a pod was created by a Job or CronJob
func isPodCreatedByJobOrCronJob(pod *corev1.Pod) (bool, string) {
	for _, ownerRef := range pod.OwnerReferences {
		if ownerRef.Kind == "Job" {
			return true, "Job"
		}
	}

	return false, ""
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

// formatRecommendationsStatus formats the recommendations for CloudCostOptimizer status
func formatRecommendationsStatus(recommendations []ResourceRecommendation) []string {
	var status []string
	for _, rec := range recommendations {
		status = append(status, fmt.Sprintf("%s/%s (%s): CPU %s->%s, Memory %s->%s",
			rec.Namespace, rec.PodName, rec.ContainerName,
			formatResourceValue(rec.CurrentRequestsCPU), formatResourceValue(rec.RecommendedCPU),
			formatResourceValue(rec.CurrentRequestsMemory), formatResourceValue(rec.RecommendedMemory)))
	}
	return status
}

// SetupWithManager sets up the controller with the Manager.
func (r *CloudCostOptimizerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&optimizationv1alpha1.CloudCostOptimizer{}).
		Complete(r)
}

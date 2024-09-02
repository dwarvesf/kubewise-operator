package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	optimizationv1alpha1 "github.com/dwarvesf/cloud-cost-optimizer/api/v1alpha1"
)

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
		// Check if the pod's owner should be ignored
		if r.shouldIgnoreResource(ctx, &pod, target.IgnoreResources) {
			logger.Info("Skipping ignored resource", "pod", pod.Name, "namespace", pod.Namespace)
			continue
		}

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

// shouldIgnoreResource checks if the resource should be ignored based on the IgnoreResources configuration
func (r *CloudCostOptimizerReconciler) shouldIgnoreResource(ctx context.Context, pod *corev1.Pod, ignoreResources map[string][]string) bool {
	logger := log.FromContext(ctx)

	for _, ownerRef := range pod.OwnerReferences {
		switch ownerRef.Kind {
		case "ReplicaSet":
			var rs appsv1.ReplicaSet
			if err := r.Get(ctx, types.NamespacedName{Name: ownerRef.Name, Namespace: pod.Namespace}, &rs); err != nil {
				logger.Error(err, "Failed to get ReplicaSet", "name", ownerRef.Name)
				return false
			}
			for _, rsOwnerRef := range rs.OwnerReferences {
				if rsOwnerRef.Kind == "Deployment" {
					if resourceNames, ok := ignoreResources["deployment"]; ok {
						for _, name := range resourceNames {
							if name == rsOwnerRef.Name {
								return true
							}
						}
					}
				}
			}
		case "StatefulSet":
			if resourceNames, ok := ignoreResources["statefulSet"]; ok {
				for _, name := range resourceNames {
					if name == ownerRef.Name {
						return true
					}
				}
			}
		case "DaemonSet":
			if resourceNames, ok := ignoreResources["daemonSet"]; ok {
				for _, name := range resourceNames {
					if name == ownerRef.Name {
						return true
					}
				}
			}
		}
	}

	return false
}

// applyOptimization applies the recommended resource changes to the Deployment, StatefulSet, or DaemonSet
func (r *CloudCostOptimizerReconciler) applyOptimization(ctx context.Context, pod *corev1.Pod, recommendations []ResourceRecommendation) error {
	logger := log.FromContext(ctx)
	logger.Info("Applying optimization", "pod", pod.Name)

	// Check if the pod is owned by a Deployment, StatefulSet, or DaemonSet
	for _, ownerRef := range pod.OwnerReferences {
		switch ownerRef.Kind {
		case "ReplicaSet":
			// For Deployments, we need to find the Deployment that owns the ReplicaSet
			var rs appsv1.ReplicaSet
			if err := r.Get(ctx, types.NamespacedName{Name: ownerRef.Name, Namespace: pod.Namespace}, &rs); err != nil {
				return fmt.Errorf("failed to get ReplicaSet: %v", err)
			}
			for _, rsOwnerRef := range rs.OwnerReferences {
				if rsOwnerRef.Kind == "Deployment" {
					return r.updateDeployment(ctx, pod.Namespace, rsOwnerRef.Name, recommendations)
				}
			}
		case "StatefulSet":
			return r.updateStatefulSet(ctx, pod.Namespace, ownerRef.Name, recommendations)
		case "DaemonSet":
			return r.updateDaemonSet(ctx, pod.Namespace, ownerRef.Name, recommendations)
		}
	}

	logger.Info("Pod is not owned by a Deployment, StatefulSet, or DaemonSet. Skipping optimization.")
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

			// Round up the recommended memory to the nearest power of 2
			recMemory = roundUpToPowerOfTwo(recMemory)

			recommendedCPU := resource.NewMilliQuantity(int64(recCPU), resource.DecimalSI)
			recommendedMemory := resource.NewQuantity(int64(recMemory), resource.BinarySI)

			if recommendedCPU.Cmp(*cpuRequest) == 0 && recommendedMemory.Cmp(*memoryRequest) == 0 {
				logger.Info("No recommendation needed", "container", container.Name)
				continue
			}

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

func roundUpToPowerOfTwo(value float64) float64 {
	return math.Pow(2, math.Ceil(math.Log2(value)))
}

// formatRecommendationsMessage formats the recommendations for a Discord message
func formatRecommendationsMessage(recommendations []ResourceRecommendation) string {
	var sb strings.Builder
	sb.WriteString("**Resource Optimization Recommendations**\n\n")

	// Group recommendations by namespace and pod
	groupedRecs := make(map[string]map[string]map[string]ResourceRecommendation)
	for _, rec := range recommendations {
		if _, ok := groupedRecs[rec.Namespace]; !ok {
			groupedRecs[rec.Namespace] = make(map[string]map[string]ResourceRecommendation)
		}
		if _, ok := groupedRecs[rec.Namespace][rec.PodName]; !ok {
			groupedRecs[rec.Namespace][rec.PodName] = make(map[string]ResourceRecommendation)
		}
		groupedRecs[rec.Namespace][rec.PodName][rec.ContainerName] = rec
	}

	// Iterate through grouped recommendations
	for namespace, pods := range groupedRecs {
		autoAdjust := ""
		// Check if all recommendations in the namespace are automated
		for _, containers := range pods {
			for _, rec := range containers {
				if rec.AutomateOptimization {
					autoAdjust = " (Auto-adjusted)"
					break
				}
			}
			if autoAdjust != "" {
				break
			}
		}
		sb.WriteString(fmt.Sprintf("**Namespace:** %s%s\n", namespace, autoAdjust))
		for podName, containers := range pods {
			sb.WriteString(fmt.Sprintf("Pod: %s\n", podName))
			for _, rec := range containers {
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

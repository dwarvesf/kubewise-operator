package controllers

import (
	"context"
	"sort"

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
	AutomateOptimization  bool
}

//+kubebuilder:rbac:groups=optimization.dwarvesf.com,resources=cloudcostoptimizers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=optimization.dwarvesf.com,resources=cloudcostoptimizers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=optimization.dwarvesf.com,resources=cloudcostoptimizers/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups=apps,resources=deployments;statefulsets;daemonsets;replicasets,verbs=get;list;watch;update;patch

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
		if target.AutomateOptimization {
			for i, _ := range targetRecommendations {
				targetRecommendations[i].AutomateOptimization = true
			}
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

// SetupWithManager sets up the controller with the Manager.
func (r *CloudCostOptimizerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&optimizationv1alpha1.CloudCostOptimizer{}).
		Complete(r)
}

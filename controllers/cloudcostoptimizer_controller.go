package controllers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	optimizationv1alpha1 "github.com/dwarvesf/kubewise-operator/api/v1alpha1"
	"github.com/dwarvesf/kubewise-operator/internal/discord"
	"github.com/dwarvesf/kubewise-operator/internal/gpt"
)

// CloudCostOptimizerReconciler reconciles a CloudCostOptimizer object
type CloudCostOptimizerReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Discord discord.DiscordService
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
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
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

	// init the discord service
	ctx = context.Background()
	// set metadata to ctx
	ctx = context.WithValue(ctx, "metadata", cloudCostOptimizer.Spec.Metadata)
	dc := discord.NewDiscordService(ctx, cloudCostOptimizer.Spec.Communication.Discord.WebhookURL, cloudCostOptimizer.Spec.Communication.Discord.BotToken)
	if dc == nil {
		return ctrl.Result{}, fmt.Errorf("unable to create Discord service: discord service is nil")
	}
	if r.Discord == nil {
		if err := dc.Bot().Open(); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to open discord bot: %w", err)
		}
		dc.Bot().SetGPT(gpt.NewOpenAI(cloudCostOptimizer.Spec.GPT.OpenAIKey))
		dc.Bot().SetK8sClient(r.Client)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CloudCostOptimizerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&optimizationv1alpha1.CloudCostOptimizer{}).
		Complete(r)
}

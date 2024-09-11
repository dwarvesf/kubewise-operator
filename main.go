package main

import (
	"flag"
	"os"
	//+kubebuilder:scaffold:imports

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	optimizationv1alpha1 "github.com/dwarvesf/kubewise-operator/api/v1alpha1"
	"github.com/dwarvesf/kubewise-operator/controllers"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(optimizationv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog.Info("Initializing manager")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: 9443,
		}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "1a6d2c1e.dwarvesf.com",
	})
	if err != nil {
		setupLog.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	reconciler := &controllers.CloudCostOptimizerReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}

	setupLog.Info("Setting up CloudCostOptimizer controller")
	if err = reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "CloudCostOptimizer")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up ready check")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()

	// Start the manager in a separate goroutine
	go func() {
		setupLog.Info("Starting manager")
		if err := mgr.Start(ctx); err != nil {
			setupLog.Error(err, "Problem running manager")
			os.Exit(1)
		}
	}()

	// Wait for the context to be cancelled (i.e., for the program to be terminated)
	<-ctx.Done()

	reconciler.Discord.Bot().Close()
}

// func initializeComponents(c client.Client, reconciler *controllers.CloudCostOptimizerReconciler) error {
// 	setupLog.Info("Creating Communication configuration secret")
// 	if err := createCommunicationConfigSecret(c); err != nil {
// 		return fmt.Errorf("unable to create Communication configuration secret: %w", err)
// 	}

// 	setupLog.Info("Reading Communication configuration from secret")
// 	communication, err := getCommunicationConfigFromSecret(c)
// 	if err != nil {
// 		return fmt.Errorf("unable to get Communication configuration from secret: %w", err)
// 	}

// 	setupLog.Info("Initializing Discord service")
// 	dc := discord.NewDiscordService(communication.Discord.WebhookURL, communication.Discord.BotToken)
// 	if dc == nil {
// 		return fmt.Errorf("unable to create Discord service: discord service is nil")
// 	}
// 	if err := dc.Bot().Open(); err != nil {
// 		return fmt.Errorf("unable to open discord bot: %w", err)
// 	}

// 	setupLog.Info("Updating controller with Discord service")
// 	if err := updateControllerWithDiscordService(reconciler, dc); err != nil {
// 		return fmt.Errorf("unable to update controller with Discord service: %w", err)
// 	}

// 	return nil
// }

// func updateControllerWithDiscordService(reconciler *controllers.CloudCostOptimizerReconciler, dc discord.DiscordService) error {
// 	reconciler.Discord = dc
// 	setupLog.Info("Controller updated with Discord service")
// 	return nil
// }

// // createCommunicationConfigSecret creates a secret to store the communication configuration
// // for the CloudCostOptimizer. This secret is used to persist communication settings across
// // restarts and to share them between different components of the system.
// func createCommunicationConfigSecret(c client.Client) error {
// 	// Check if the secret already exists
// 	existingSecret := &corev1.Secret{}
// 	err := c.Get(context.Background(), types.NamespacedName{
// 		Name:      "cloudcostoptimizer-communication-config",
// 		Namespace: "kubewise-operator-system",
// 	}, existingSecret)

// 	if err == nil {
// 		setupLog.Info("Communication configuration secret already exists")
// 		return nil
// 	}

// 	if !errors.IsNotFound(err) {
// 		return fmt.Errorf("failed to check for existing secret: %w", err)
// 	}

// 	setupLog.Info("Communication configuration secret not found, creating new one")

// 	// Get the CloudCostOptimizer instances
// 	ccoList := &optimizationv1alpha1.CloudCostOptimizerList{}
// 	if err := c.List(context.Background(), ccoList, client.InNamespace("kubewise-operator-system")); err != nil {
// 		return fmt.Errorf("failed to list CloudCostOptimizer instances: %w", err)
// 	}

// 	if len(ccoList.Items) == 0 {
// 		return fmt.Errorf("no CloudCostOptimizer instances found")
// 	}

// 	// Use the first CloudCostOptimizer instance found
// 	cco := &ccoList.Items[0]

// 	// Get the Communication config from CloudCostOptimizerSpec
// 	communicationConfig := cco.Spec.Communication

// 	configJSON, err := json.Marshal(communicationConfig)
// 	if err != nil {
// 		return fmt.Errorf("failed to marshal Communication config: %w", err)
// 	}

// 	secret := &corev1.Secret{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      "cloudcostoptimizer-communication-config",
// 			Namespace: "kubewise-operator-system",
// 		},
// 		Data: map[string][]byte{
// 			"communication_config": configJSON,
// 		},
// 	}

// 	if err := c.Create(context.Background(), secret); err != nil {
// 		return fmt.Errorf("failed to create Communication config secret: %w", err)
// 	}

// 	setupLog.Info("Created Communication configuration secret")
// 	return nil
// }

// func getCommunicationConfigFromSecret(c client.Client) (*optimizationv1alpha1.Communication, error) {
// 	secret := &corev1.Secret{}
// 	err := c.Get(context.Background(), types.NamespacedName{
// 		Name:      "cloudcostoptimizer-communication-config",
// 		Namespace: "kubewise-operator-system",
// 	}, secret)

// 	if err != nil {
// 		if errors.IsNotFound(err) {
// 			setupLog.Info("Communication configuration secret not found, attempting to create it")
// 			if createErr := createCommunicationConfigSecret(c); createErr != nil {
// 				return nil, fmt.Errorf("failed to create communication config secret: %w", createErr)
// 			}
// 			// Try to get the secret again
// 			err = c.Get(context.Background(), types.NamespacedName{
// 				Name:      "cloudcostoptimizer-communication-config",
// 				Namespace: "kubewise-operator-system",
// 			}, secret)
// 			if err != nil {
// 				return nil, fmt.Errorf("failed to get communication config secret after creation: %w", err)
// 			}
// 		} else {
// 			return nil, fmt.Errorf("failed to get communication config secret: %w", err)
// 		}
// 	}

// 	communicationConfigData, ok := secret.Data["communication_config"]
// 	if !ok {
// 		return nil, fmt.Errorf("communication_config not found in secret")
// 	}

// 	var communicationConfig optimizationv1alpha1.Communication
// 	if err := json.Unmarshal(communicationConfigData, &communicationConfig); err != nil {
// 		return nil, fmt.Errorf("failed to unmarshal communication config: %w", err)
// 	}

// 	setupLog.Info("Successfully retrieved Communication configuration from secret")
// 	return &communicationConfig, nil
// }

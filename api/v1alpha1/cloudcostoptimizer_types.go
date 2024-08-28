package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CloudCostOptimizerSpec defines the desired state of CloudCostOptimizer
type CloudCostOptimizerSpec struct {
	// Targets specifies the resources to analyze and optimize
	// +kubebuilder:validation:Required
	Targets []Target `json:"targets"`

	// AnalysisInterval specifies how often to run the optimization analysis
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=duration
	AnalysisInterval metav1.Duration `json:"analysisInterval"`

	// CostSavingThreshold specifies the minimum cost saving percentage to trigger an action
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	CostSavingThreshold int `json:"costSavingThreshold,omitempty"`

	// PrometheusConfig specifies the configuration for Prometheus
	// +optional
	PrometheusConfig PrometheusConfig `json:"prometheusConfig,omitempty"`

	// DiscordConfig specifies the configuration for Discord notifications
	// +optional
	DiscordConfig DiscordConfig `json:"discordConfig,omitempty"`
}

// Target defines the resources and namespaces to analyze and optimize
type Target struct {
	// Resources specifies the types of resources to analyze and optimize
	// +kubebuilder:validation:Required
	Resources []string `json:"resources"`

	// Namespaces specifies the list of namespaces to monitor
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

	// AutomateOptimization specifies whether to automatically apply optimization recommendations
	// +optional
	AutomateOptimization bool `json:"automateOptimization,omitempty"`
}

// PrometheusConfig defines the configuration for Prometheus
type PrometheusConfig struct {
	// ServerAddress is the address of the Prometheus server
	// +kubebuilder:validation:Required
	ServerAddress string `json:"serverAddress"`

	// HistoricalMetricDuration specifies the duration for historical metric analysis
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=duration
	HistoricalMetricDuration metav1.Duration `json:"historicalMetricDuration"`
}

// DiscordConfig defines the configuration for Discord notifications
type DiscordConfig struct {
	// WebhookURL is the Discord webhook URL for sending notifications
	// +kubebuilder:validation:Required
	WebhookURL string `json:"webhookURL"`
}

// CloudCostOptimizerStatus defines the observed state of CloudCostOptimizer
type CloudCostOptimizerStatus struct {
	// LastAnalysisTime is the timestamp of the last optimization analysis
	// +optional
	LastAnalysisTime metav1.Time `json:"lastAnalysisTime,omitempty"`

	// LastReconcileTime is the timestamp of the last reconciliation
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// Recommendations contains the latest optimization recommendations
	// +optional
	Recommendations []string `json:"recommendations,omitempty"`

	// RecommendationsHash contains the hash of the latest optimization recommendations
	// +optional
	RecommendationsHash string `json:"recommendationsHash,omitempty"`

	// TotalCostSavings represents the estimated total cost savings
	// +optional
	TotalCostSavings string `json:"totalCostSavings,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CloudCostOptimizer is the Schema for the cloudcostoptimizers API
type CloudCostOptimizer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudCostOptimizerSpec   `json:"spec,omitempty"`
	Status CloudCostOptimizerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CloudCostOptimizerList contains a list of CloudCostOptimizer
type CloudCostOptimizerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudCostOptimizer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudCostOptimizer{}, &CloudCostOptimizerList{})
}

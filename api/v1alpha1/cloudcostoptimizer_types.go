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

	// RecommendationThreshold specifies the minimum percentage difference between current and recommended resources to generate a recommendation
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:validation:Default=10
	RecommendationThreshold int `json:"recommendationThreshold,omitempty"`

	// OOMKilledMemoryIncreasePercentage specifies the percentage to increase memory when a pod is OOMKilled
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:validation:Default=20
	OOMKilledMemoryIncreasePercentage int `json:"oomKilledMemoryIncreasePercentage,omitempty"`

	// HighCPUUsageThreshold specifies the CPU usage percentage threshold to consider as high CPU usage
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:validation:Default=90
	HighCPUUsageThreshold int `json:"highCPUUsageThreshold,omitempty"`

	// HighCPUUsageDuration specifies the duration for which CPU usage should be high to trigger an increase
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=duration
	HighCPUUsageDuration metav1.Duration `json:"highCPUUsageDuration,omitempty"`

	// HighCPUUsageIncreasePercentage specifies the percentage to increase CPU when high CPU usage is detected
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:validation:Default=20
	HighCPUUsageIncreasePercentage int `json:"highCPUUsageIncreasePercentage,omitempty"`

	// PrometheusConfig specifies the configuration for Prometheus
	// +optional
	PrometheusConfig PrometheusConfig `json:"prometheusConfig,omitempty"`

	// Communication specifies the configuration for Discord notifications
	// +optional
	Communication Communication `json:"communication,omitempty"`

	// GPT specifies the configuration for AI
	// +optional
	GPT GPT `json:"gpt,omitempty"`

	// Metadata specifies the metadata for the CloudCostOptimizer instance
	// +optional
	Metadata map[string]string `json:"metadata,omitempty"`
}

type GPT struct {
	// OpenAIKey is the OpenAI API key for AI-based recommendations
	// +kubebuilder:validation:Required
	OpenAIKey string `json:"openAIKey"`
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

	// IgnoreResources specifies resources to ignore by their names, grouped by resource type
	// +optional
	// +kubebuilder:validation:Type=object
	// +kubebuilder:validation:AdditionalProperties=true
	IgnoreResources map[string][]string `json:"ignoreResources,omitempty"`
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

type Communication struct {
	// Discord specifies the configuration for Discord notifications
	// +kubebuilder:validation:Required
	Discord DiscordConfig `json:"discord"`
}

// DiscordConfig defines the configuration for Discord notifications
type DiscordConfig struct {
	// WebhookURL is the Discord webhook URL for sending notifications
	// +kubebuilder:validation:Required
	WebhookURL string `json:"webhookURL"`
	// BotToken is the Discord bot token for sending notifications
	// +kubebuilder:validation:Required
	BotToken string `json:"botToken"`
}

// CloudCostOptimizerStatus defines the observed state of CloudCostOptimizer
type CloudCostOptimizerStatus struct {
	// LastReconcileTime is the timestamp of the last reconciliation
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// DiscordStatus is the status of the Discord service
	// +optional
	DiscordStatus string `json:"discordStatus,omitempty"`
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

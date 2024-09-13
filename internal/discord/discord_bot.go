// discord_bot.go
package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/olekukonko/tablewriter"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dwarvesf/kubewise-operator/internal/gpt"
)

type K8sCommand struct {
	Operation     string   `json:"operation"`
	Resource      string   `json:"resource"`
	Namespace     string   `json:"namespace"`
	LabelSelector []string `json:"label-selector"`
	FieldSelector []string `json:"field-selector"`
	AllNamespaces bool     `json:"all-namespaces"`
}

type dcBot struct {
	s         *discordgo.Session
	gpt       gpt.GPT
	handler   func(s *discordgo.Session, m *discordgo.MessageCreate)
	k8sClient client.Client
	ctx       context.Context
}

func NewDiscordBot(ctx context.Context, token string) DiscordBot {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatal(err)
	}

	return &dcBot{s: dg, ctx: ctx}
}

func (db *dcBot) SetGPT(gpt gpt.GPT) {
	db.gpt = gpt
}

func (db *dcBot) SetK8sClient(client client.Client) {
	db.k8sClient = client
}

func (db *dcBot) Open() error {
	// Add a handler to log when the bot is ready
	db.s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
	})

	// Add a handler to process messages
	db.s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Ignore messages from the bot itself
		if m.Author.ID == s.State.User.ID {
			return
		}

		log.Printf("Message received: %s", m.Content)

		// Check if the bot was mentioned in the message
		if mentioned(s, m) {
			// Remove the bot mention from the message content
			cmd := stripBotMention(s, m)

			// Process the command
			db.ProcessCommand(s, m, cmd)
		}
	})

	// Open the WebSocket connection to Discord
	if err := db.s.Open(); err != nil {
		return fmt.Errorf("failed to open Discord session: %w", err)
	}

	return nil
}

// Helper function to check if the bot was mentioned in the message
func mentioned(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	for _, user := range m.Mentions {
		if user.ID == s.State.User.ID {
			return true
		}
	}
	return false
}

// Helper function to strip the bot mention from the message content
func stripBotMention(s *discordgo.Session, m *discordgo.MessageCreate) string {
	botMention := fmt.Sprintf("<@%s>", s.State.User.ID)
	cmd := strings.Replace(m.Content, botMention, "", 1)
	return strings.TrimSpace(cmd)
}

func (db *dcBot) Close() error {
	return db.s.Close()
}

func (db *dcBot) AddHandler(handler func(s *discordgo.Session, m *discordgo.MessageCreate)) {
	db.handler = handler
}

type Response struct {
	Query             string `json:"query"`
	Response          string `json:"response"`
	Type              string `json:"type"`
	ResourcesAnalyzed []struct {
		Name      string `json:"name"`
		Type      string `json:"type"`
		Namespace string `json:"namespace"`
		Age       string `json:"age"`
		Status    string `json:"status"`
		Metadata  struct {
			IP           string `json:"ip"`
			Node         string `json:"node"`
			RestartCount int    `json:"restart_count"`
			LastState    struct {
				StartedAt string `json:"started_at"`
				Reason    string `json:"reason"`
			} `json:"last_state"`
		} `json:"metadata"`
	} `json:"resources_analyzed"`
	Recommendations []string `json:"recommendations"`
	Errors          []any    `json:"errors"`
}

func (db *dcBot) ProcessCommand(s *discordgo.Session, m *discordgo.MessageCreate, cmd string) {
	// Split the command into individual words
	words := strings.Split(cmd, " ")

	if len(words) == 1 {
		// Handle single-word commands
		switch words[0] {
		case "help":
			db.HelpCommand(s, m)
		default:
			db.UnknownCommand(s, m)
		}
		return
	}

	if len(words) >= 2 {
		switch words[0] {
		case "restart", "rs":
			db.RestartPodCommand(s, m, cmd)
			return
		}
	}

	metadataMap := db.ctx.Value("metadata").(map[string]string)
	metadata, err := json.Marshal(metadataMap)
	if err != nil {
		db.SendMessage(m.ChannelID, fmt.Sprintf("Error parsing metadata: %v", err))
		return
	}

	resources, err := db.getAllResources()
	if err != nil {
		db.SendMessage(m.ChannelID, fmt.Sprintf("Error getting resources: %v", err))
		return
	}
	log.Println("ressource length: ", len(resources))

	prompt := fmt.Sprintf(`You are an expert DevOps engineer with extensive knowledge of Kubernetes (K8s). Your task is to analyze Kubernetes resources and respond to user queries about them. You will be provided with all resources in a Kubernetes cluster for analysis. Your responses should be in JSON format.

Here are the Kubernetes resources you have access to:

<k8s_resources>
%v
</k8s_resources>

Here are additional information that you can use to answer user queries:
<additional_info>
%v
</additional_info>

When a user submits a query, analyze the provided Kubernetes resources and formulate a response based on your expertise. Your response should be in the following JSON format:

{
  "query": "The user's original query",
  "response": "Your detailed response to the query",
  "resources_analyzed": ["List of specific resources you analyzed to answer the query in JSON format eg: [{\"name\": \"pod-1\", \"type\": \"Pod\", \"namespace\": \"default\", \"age\": \"2h\", \"status\": \"Running\", \"metadata\": {\"ip\": \"1.2.3.4\"}]"],
  "recommendations": ["Any recommendations or best practices, if applicable"],
  "errors": ["Any errors or issues you encountered, if any"]
}

To process the user's query, follow these steps:
1. Carefully read and understand the user's query.
2. Identify the relevant Kubernetes resources that need to be analyzed to answer the query.
3. Analyze the identified resources and extract the necessary information.
4. Make sure responses are accurate, concise, and won't miss any information in tag <k8s_resources>.
4. Formulate a clear and concise response that directly addresses the user's query.
5. If applicable, provide recommendations or best practices related to the query.
6. If you encounter any errors or issues while processing the query, include them in the "errors" field.
7. If the question is not about the resources provided, respond with information that is given in the <additional_info> tag and put it into the "response" field, and "type" field is "general".
8. Response should be in text, DON'T inlcude any quotes in the response.

Here are some examples of possible queries and how you should structure your responses:

Query: "How many pods are running in the default namespace?"
Response:
{
  "query": "How many pods are running in the default namespace?",
  "response": "There are 5 pods running in the default namespace.",
  "resources_analyzed": ["pods in default namespace"],
  "type": "k8s",
  "recommendations": ["Consider using namespaces to organize and isolate your resources for better management."],
  "errors": []
}

Query: "What is the CPU usage of the nginx deployment?"
Response:
{
  "query": "What is the CPU usage of the nginx deployment?",
  "response": "The nginx deployment is currently using 250m CPU across its 3 replicas.",
  "resources_analyzed": ["nginx deployment", "pods in nginx deployment"],
  "type": "k8s",
  "recommendations": ["Monitor CPU usage regularly and consider setting resource limits and requests for better resource management."],
  "errors": []
}

Query: "What is cluster name?"
Response:
{
  "query": "What is cluster name?",
  "response": "The cluster name is 'my-cluster'.",
  "type": "general",
  "errors": [],
  "resources_analyzed": [],
  "recommendations": []
}

If you encounter a query that cannot be answered with the provided Kubernetes resources or requires information outside of your knowledge base, respond with an appropriate error message in the "errors" field.

Remember to always provide accurate and helpful information based on the given Kubernetes resources. Do not make assumptions about resources or configurations that are not explicitly provided in the {{K8S_RESOURCES}}.

Now, please analyze the Kubernetes resources and respond to the following user query:

<user_query>
{{USER_QUERY}}
</user_query>

Provide your response in the specified JSON format.`, resources, string(metadata))
	log.Println("prompt: ", prompt)

	// Handle multi-word commands
	log.Println("GPT request")
	resp, err := db.gpt.Request(prompt, cmd)
	if err != nil {
		db.SendMessage(m.ChannelID, fmt.Sprintf("Error processing command: %v", err))
		return
	}
	if strings.Contains(resp, "```json") {
		log.Println("processing response")
		resp = strings.ReplaceAll(resp, "```json", "")
		resp = strings.ReplaceAll(resp, "```", "")
	}

	var response Response
	if err := json.Unmarshal([]byte(resp), &response); err != nil {
		db.SendMessage(m.ChannelID, fmt.Sprintf("Error parsing GPT response: %v", err))
		return
	}
	log.Println("response: ", response)

	// use table writer to format the resources analyzed
	tableString := &strings.Builder{}
	table := tablewriter.NewWriter(tableString)

	// Flags to track which headers need to be set
	podHeaderSet := false
	namespaceHeaderSet := false

	log.Println("processing resources")
	// sort the resources analyzed by namespace, then by name
	sort.Slice(response.ResourcesAnalyzed, func(i, j int) bool {
		if response.ResourcesAnalyzed[i].Namespace == response.ResourcesAnalyzed[j].Namespace {
			return response.ResourcesAnalyzed[i].Name < response.ResourcesAnalyzed[j].Name
		}
		return response.ResourcesAnalyzed[i].Namespace < response.ResourcesAnalyzed[j].Namespace
	})
	for _, res := range response.ResourcesAnalyzed {
		if res.Type == "Pod" {
			// Set the header for Pod if not already set
			if !podHeaderSet {
				table.SetHeader([]string{"Name", "Namespace", "Status", "Restart", "IP", "Age"})
				podHeaderSet = true
			}
			restartReason := ""
			if res.Metadata.LastState.Reason != "" {
				restartReason = fmt.Sprintf(" (%v)", res.Metadata.LastState.Reason)
			}
			table.Append([]string{res.Name, res.Namespace, res.Status, fmt.Sprintf("%v", res.Metadata.RestartCount) + restartReason, res.Metadata.IP, res.Age})
		} else if res.Type == "Namespace" {
			// Set the header for Namespace if not already set
			if !namespaceHeaderSet {
				table.SetHeader([]string{"Name", "Age"})
				namespaceHeaderSet = true
			}
			table.Append([]string{res.Name, res.Age})
		}
	}

	// Render the table only if there are rows added for either type
	if podHeaderSet || namespaceHeaderSet {
		table.Render()

		// Send the result back to the user
		db.SendMessage(m.ChannelID, fmt.Sprintf("%s:\n```\n%s\n```", response.Response, tableString.String()))
	} else {
		if response.Type == "general" {
			db.SendMessage(m.ChannelID, response.Response)
			return
		}
		// Handle the case when no relevant resources were found
		if len(response.ResourcesAnalyzed) == 0 {
			db.SendMessage(m.ChannelID, "No resources of type 'Pod' or 'Namespace' found.")
		}
	}
}

// Helper function to convert duration to human-readable format
func humanReadableDuration(d time.Duration) string {
	// Calculate days, hours, minutes, and seconds from the duration
	days := d / (24 * time.Hour)
	d -= days * (24 * time.Hour)

	hours := d / time.Hour
	d -= hours * time.Hour

	minutes := d / time.Minute
	d -= minutes * time.Minute

	// Build the human-readable format
	var result string
	if days > 0 {
		result += fmt.Sprintf("%dd ", days)
	}
	if hours > 0 {
		result += fmt.Sprintf("%dh ", hours)
	}
	if days == 0 && hours == 0 {
		if minutes > 0 {
			result += fmt.Sprintf("%dm ", minutes)
		}
	}
	if days == 0 && hours == 0 && minutes == 0 {
		seconds := d / time.Second
		if seconds > 0 || result == "" { // Show seconds even if they're zero when nothing else is shown
			result += fmt.Sprintf("%ds", seconds)
		}
	}

	return result
}

type resource struct {
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Namespace string                 `json:"namespace"`
	Age       string                 `json:"age"`
	Status    string                 `json:"status"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// Function to fetch and return all resources as a formatted string
func (db *dcBot) getAllResources() (string, error) {
	start := time.Now()
	var resources []resource
	var namespaceList corev1.NamespaceList
	if err := db.k8sClient.List(context.Background(), &namespaceList); err != nil {
		return "", err
	}
	for _, ns := range namespaceList.Items {
		resources = append(resources, resource{
			Name:      ns.Name,
			Type:      "Namespace",
			Namespace: "",
			Age:       time.Since(ns.CreationTimestamp.Time).Round(time.Second).String(),
			Metadata:  map[string]interface{}{},
		})
	}

	// Fetch and list Pods
	listOptions := &client.ListOptions{}
	var pods corev1.PodList
	if err := db.k8sClient.List(context.Background(), &pods, listOptions); err != nil {
		return "", err
	}
	namespacePods := make(map[string][]resource)
	for _, pod := range pods.Items {
		age := time.Since(pod.CreationTimestamp.Time).Round(time.Second)
		// fetch node external ip (if any) from node name
		externalIP := "<none>"
		if pod.Spec.NodeName != "" {
			var node corev1.Node
			if err := db.k8sClient.Get(context.Background(), client.ObjectKey{Name: pod.Spec.NodeName}, &node); err == nil {
				for _, address := range node.Status.Addresses {
					if address.Type == corev1.NodeExternalIP {
						externalIP = address.Address
						break
					}
				}
			}
		}

		// Get restart count and last state
		restartCount := 0
		var lastState corev1.ContainerState
		if len(pod.Status.ContainerStatuses) > 0 {
			restartCount = int(pod.Status.ContainerStatuses[0].RestartCount)
			lastState = pod.Status.ContainerStatuses[0].LastTerminationState
		}

		lastStateInfo := map[string]string{
			"started_at": "",
			"reason":     "",
		}
		if lastState.Terminated != nil {
			lastStateInfo["started_at"] = lastState.Terminated.StartedAt.String()
			lastStateInfo["reason"] = lastState.Terminated.Reason
		}

		namespacePods[pod.Namespace] = append(namespacePods[pod.Namespace], resource{
			Name:      pod.Name,
			Type:      "Pod",
			Namespace: pod.Namespace,
			Age:       humanReadableDuration(age),
			Status:    string(pod.Status.Phase),
			Metadata: map[string]interface{}{
				"ip":            externalIP,
				"node":          pod.Spec.NodeName,
				"restart_count": restartCount,
				"last_state":    lastStateInfo,
			},
		})
	}
	// Add the pods to the resources slice
	for i := range resources {
		resources[i].Metadata = map[string]interface{}{"pods": namespacePods[resources[i].Name]}
	}
	log.Printf("Fetched all resources in %v seconds", time.Since(start).Seconds())

	// marshal the resources to JSON
	result, err := json.Marshal(resources)
	if err != nil {
		return "", err
	}

	return string(result), nil
}

func (db *dcBot) RestartPodCommand(s *discordgo.Session, m *discordgo.MessageCreate, cmd string) {
	// Split the command into individual words
	words := strings.Split(cmd, " ")
	pod := words[1]
	namespace := words[2]
	if err := db.k8sClient.Delete(context.Background(), &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: pod, Namespace: namespace}}); err != nil {
		db.SendMessage(m.ChannelID, fmt.Sprintf("Error restarting pod: %v", err))
		return
	}
	db.SendMessage(m.ChannelID, fmt.Sprintf("Pod %s in namespace %s restarted successfully", pod, namespace))
}

func (db *dcBot) UnknownCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	db.SendMessage(m.ChannelID, "Unknown command. Try using the 'help' command for more information.")
}

func (db *dcBot) SendMessage(channelID string, content string) (*discordgo.Message, error) {
	return db.s.ChannelMessageSend(channelID, content)
}

func (db *dcBot) HelpCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	helpMessage := `Available commands:
- help: Show this help message
- Any other message: Will be processed by GPT and executed as a Kubernetes command

Examples:
- Get pods: "List all pods in the default namespace"
- Describe a service: "Describe the nginx service in the kube-system namespace"
- Get nodes: "Show me all the nodes in the cluster"
- Complex query: "Get pods in kube-system namespace that are not in Running state"
- Delete resource: "Delete the deployment named 'my-app' in the 'production' namespace"
- All namespaces: "List pods in all namespaces"

The GPT model will interpret your request and generate the appropriate Kubernetes command.`

	db.SendMessage(m.ChannelID, helpMessage)
}

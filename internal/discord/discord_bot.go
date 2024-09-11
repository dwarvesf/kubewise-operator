// discord_bot.go
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/olekukonko/tablewriter"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
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
}

func NewDiscordBot(token string) DiscordBot {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatal(err)
	}

	return &dcBot{s: dg}
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

	// Handle multi-word commands
	k8sCmdStr, err := db.gpt.QueryToCommand(cmd)
	if err != nil {
		db.SendMessage(m.ChannelID, fmt.Sprintf("Error processing command: %v", err))
		return
	}
	log.Printf("Kubernetes command string: %s", k8sCmdStr)
	k8sCmdStr = strings.ReplaceAll(k8sCmdStr, "`", "")

	var k8sCmd K8sCommand
	err = json.Unmarshal([]byte(k8sCmdStr), &k8sCmd)
	if err != nil {
		db.SendMessage(m.ChannelID, fmt.Sprintf("Error parsing Kubernetes command: %v", err))
		return
	}
	log.Printf("Kubernetes command: %+v", k8sCmd)

	// Execute the Kubernetes command
	result, err := db.executeK8sCommand(k8sCmd)
	if err != nil {
		db.SendMessage(m.ChannelID, fmt.Sprintf("Error executing Kubernetes command: %v", err))
		return
	}

	// Send the result back to the user
	db.SendMessage(m.ChannelID, result)
}

func (db *dcBot) executeK8sCommand(cmd K8sCommand) (string, error) {
	ctx := context.Background()

	options := commandOptions{
		namespace:     cmd.Namespace,
		labelSelector: strings.Join(cmd.LabelSelector, ","),
		fieldSelector: strings.Join(cmd.FieldSelector, ","),
		allNamespaces: cmd.AllNamespaces,
	}

	switch cmd.Operation {
	case "get":
		return db.getResource(ctx, cmd.Resource, options)
	case "describe":
		return db.describeResource(ctx, cmd.Resource, options)
	case "delete":
		return db.deleteResource(ctx, cmd.Resource, options)
	default:
		return "", fmt.Errorf("unsupported operation: %s", cmd.Operation)
	}
}

type commandOptions struct {
	namespace     string
	name          string
	fieldSelector string
	labelSelector string
	allNamespaces bool
}

func (db *dcBot) getResource(ctx context.Context, resource string, options commandOptions) (string, error) {
	switch resource {
	case "pods", "pod":
		return db.getPods(ctx, options)
	case "services", "service":
		return db.getServices(ctx, options)
	case "nodes", "node":
		return db.getNodes(ctx, options)
	case "namespace", "namespaces":
		return db.getNamespaces(ctx)
	default:
		return "", fmt.Errorf("unsupported resource type: %s", resource)
	}
}

func (db *dcBot) getNamespaces(ctx context.Context) (string, error) {
	var namespaceList corev1.NamespaceList
	if err := db.k8sClient.List(ctx, &namespaceList); err != nil {
		return "", err
	}
	if len(namespaceList.Items) == 0 {
		return "No namespaces found in the cluster", nil
	}

	buf := &bytes.Buffer{}
	table := tablewriter.NewWriter(buf)
	table.SetHeader([]string{"Name", "Status", "Age"})

	for _, ns := range namespaceList.Items {
		age := time.Since(ns.CreationTimestamp.Time).Round(time.Second).String()
		table.Append([]string{ns.Name, string(ns.Status.Phase), age})
	}
	table.Render()

	return fmt.Sprintf("Namespaces in the cluster:\n```\n%s\n```", buf.String()), nil
}

func (db *dcBot) getPods(ctx context.Context, options commandOptions) (string, error) {
	listOptions := &client.ListOptions{}

	if !options.allNamespaces {
		listOptions.Namespace = options.namespace
	}

	if options.labelSelector != "" {
		labelSelector, err := labels.Parse(options.labelSelector)
		if err != nil {
			return "", fmt.Errorf("invalid label selector: %v", err)
		}
		listOptions.LabelSelector = labelSelector
	}

	var podList corev1.PodList
	if err := db.k8sClient.List(ctx, &podList, listOptions); err != nil {
		return "", err
	}
	if len(podList.Items) == 0 {
		return fmt.Sprintf("No pods found in %s", options.namespace), nil
	}

	// Manual filtering for field selectors
	filteredPods := filterPods(podList.Items, options.fieldSelector)

	buf := &bytes.Buffer{}
	table := tablewriter.NewWriter(buf)
	table.SetHeader([]string{"Name", "Namespace", "Status", "Node", "Age"})

	for _, pod := range filteredPods {
		age := time.Since(pod.CreationTimestamp.Time).Round(time.Second).String()
		table.Append([]string{pod.Name, pod.Namespace, string(pod.Status.Phase), pod.Spec.NodeName, age})
	}
	table.Render()

	if options.allNamespaces {
		return fmt.Sprintf("Pods in all namespaces:\n```\n%s\n```", buf.String()), nil
	}
	return fmt.Sprintf("Pods in namespace %s:\n```\n%s\n```", options.namespace, buf.String()), nil
}

func (db *dcBot) getServices(ctx context.Context, options commandOptions) (string, error) {
	listOptions := &client.ListOptions{}

	if !options.allNamespaces {
		listOptions.Namespace = options.namespace
	}

	if options.labelSelector != "" {
		labelSelector, err := labels.Parse(options.labelSelector)
		if err != nil {
			return "", fmt.Errorf("invalid label selector: %v", err)
		}
		listOptions.LabelSelector = labelSelector
	}

	var serviceList corev1.ServiceList
	if err := db.k8sClient.List(ctx, &serviceList, listOptions); err != nil {
		return "", err
	}

	buf := &bytes.Buffer{}
	table := tablewriter.NewWriter(buf)
	table.SetHeader([]string{"Name", "Namespace", "Type", "ClusterIP", "External IP", "Ports", "Age"})

	for _, service := range serviceList.Items {
		age := time.Since(service.CreationTimestamp.Time).Round(time.Second).String()
		externalIP := "<none>"
		if len(service.Status.LoadBalancer.Ingress) > 0 {
			externalIP = service.Status.LoadBalancer.Ingress[0].IP
		}
		ports := []string{}
		for _, port := range service.Spec.Ports {
			ports = append(ports, fmt.Sprintf("%d/%s", port.Port, port.Protocol))
		}
		table.Append([]string{
			service.Name,
			service.Namespace,
			string(service.Spec.Type),
			service.Spec.ClusterIP,
			externalIP,
			strings.Join(ports, ", "),
			age,
		})
	}
	table.Render()

	if options.allNamespaces {
		return fmt.Sprintf("Services in all namespaces:\n```\n%s\n```", buf.String()), nil
	}
	return fmt.Sprintf("Services in namespace %s:\n```\n%s\n```", options.namespace, buf.String()), nil
}

func (db *dcBot) getNodes(ctx context.Context, options commandOptions) (string, error) {
	var nodeList corev1.NodeList
	if err := db.k8sClient.List(ctx, &nodeList); err != nil {
		return "", err
	}

	buf := &bytes.Buffer{}
	table := tablewriter.NewWriter(buf)
	table.SetHeader([]string{"Name", "Status", "Roles", "Age", "Version"})

	for _, node := range nodeList.Items {
		age := time.Since(node.CreationTimestamp.Time).Round(time.Second).String()
		table.Append([]string{
			node.Name,
			getNodeStatus(node),
			getNodeRoles(node),
			age,
			node.Status.NodeInfo.KubeletVersion,
		})
	}
	table.Render()

	return fmt.Sprintf("Nodes in the cluster:\n```\n%s\n```", buf.String()), nil
}

func filterPods(pods []corev1.Pod, fieldSelector string) []corev1.Pod {
	if fieldSelector == "" {
		return pods
	}

	var filteredPods []corev1.Pod
	for _, pod := range pods {
		if evaluateFieldSelector(pod, fieldSelector) {
			filteredPods = append(filteredPods, pod)
		}
	}
	return filteredPods
}

func evaluateFieldSelector(pod corev1.Pod, fieldSelector string) bool {
	selectors := strings.Split(fieldSelector, ",")
	for _, selector := range selectors {
		parts := strings.Split(selector, " ")
		if len(parts) != 3 {
			// Invalid selector format, skip this selector
			continue
		}

		field := parts[0]
		operator := parts[1]
		value := parts[2]

		switch field {
		case "status.phase":
			if !evaluatePhase(string(pod.Status.Phase), operator, value) {
				return false
			}
		case "metadata.name":
			if !evaluateString(pod.Name, operator, value) {
				return false
			}
		case "metadata.namespace":
			if !evaluateString(pod.Namespace, operator, value) {
				return false
			}
		case "spec.nodeName":
			if !evaluateString(pod.Spec.NodeName, operator, value) {
				return false
			}
		case "status.podIP":
			if !evaluateString(pod.Status.PodIP, operator, value) {
				return false
			}
		case "status.startTime":
			if pod.Status.StartTime != nil {
				if !evaluateTime(pod.Status.StartTime.Time, operator, value) {
					return false
				}
			} else if operator != "=" && operator != "==" {
				return false
			}
		default:
			// Unknown field, skip this selector
			continue
		}
	}
	return true
}

func evaluatePhase(actual, operator, expected string) bool {
	switch operator {
	case "=", "==":
		return actual == expected
	case "!=":
		return actual != expected
	default:
		return false
	}
}

func evaluateString(actual, operator, expected string) bool {
	switch operator {
	case "=", "==":
		return actual == expected
	case "!=":
		return actual != expected
	case "contains":
		return strings.Contains(actual, expected)
	case "startswith":
		return strings.HasPrefix(actual, expected)
	case "endswith":
		return strings.HasSuffix(actual, expected)
	default:
		return false
	}
}

func evaluateTime(actual time.Time, operator, expected string) bool {
	expectedTime, err := time.Parse(time.RFC3339, expected)
	if err != nil {
		return false
	}

	switch operator {
	case "=", "==":
		return actual.Equal(expectedTime)
	case "!=":
		return !actual.Equal(expectedTime)
	case ">":
		return actual.After(expectedTime)
	case ">=":
		return actual.After(expectedTime) || actual.Equal(expectedTime)
	case "<":
		return actual.Before(expectedTime)
	case "<=":
		return actual.Before(expectedTime) || actual.Equal(expectedTime)
	default:
		return false
	}
}

func getNodeStatus(node corev1.Node) string {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			if condition.Status == corev1.ConditionTrue {
				return "Ready"
			} else {
				return "Not Ready"
			}
		}
	}
	return "Unknown"
}

func getNodeRoles(node corev1.Node) string {
	roles := []string{}
	for label := range node.Labels {
		if strings.HasPrefix(label, "node-role.kubernetes.io/") {
			roles = append(roles, strings.TrimPrefix(label, "node-role.kubernetes.io/"))
		}
	}
	if len(roles) == 0 {
		return "none"
	}
	return strings.Join(roles, ", ")
}

func (db *dcBot) describeResource(ctx context.Context, resource string, options commandOptions) (string, error) {
	// For simplicity, we'll just return the same information as 'get' for now
	// In a real-world scenario, you'd want to provide more detailed information
	return db.getResource(ctx, resource, options)
}

func (db *dcBot) deleteResource(ctx context.Context, resource string, options commandOptions) (string, error) {
	// Implement delete logic here
	// This is a placeholder and should be implemented based on your requirements
	return fmt.Sprintf("Delete operation for %s in namespace %s is not implemented yet", resource, options.namespace), nil
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

func (db *dcBot) UnknownCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	db.SendMessage(m.ChannelID, "Unknown command. Try using the 'help' command for more information.")
}

func (db *dcBot) SendMessage(channelID string, content string) (*discordgo.Message, error) {
	return db.s.ChannelMessageSend(channelID, content)
}

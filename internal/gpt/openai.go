package gpt

import (
	"context"
	"fmt"

	"github.com/sashabaranov/go-openai"
)

type OpenAI struct {
	client *openai.Client
}

func NewOpenAI(apiKey string) GPT {
	return &OpenAI{
		client: openai.NewClient(apiKey),
	}
}

func (o *OpenAI) QueryToCommand(query string) (string, error) {
	prompt := `You are an expert DevOps engineer with extensive knowledge of Kubernetes (K8s). Your task is to translate a user's query into a Kubernetes command and return it in a specific JSON format.

Here is the user's query:
<user_query>
{{USER_QUERY}}
</user_query>

Analyze the query carefully to understand the user's intent. Identify the following elements:
1. The operation to be performed (e.g., get, describe, delete)
2. The resource type (e.g., pods, services, deployments)
3. The namespace (if specified)
4. Any label selectors
5. Any field selectors

Translate the query into a Kubernetes command based on your analysis. Then, format your response as a JSON object with the following structure:

{
  "operation": "COMMAND",
  "resource": "RESOURCE",
  "namespace": "NAMESPACE",
  "label-selector": ["label1=val1", "label2=val2"],
  "field-selector": ["field1=val1", "field2=val2"]
}

Notes:
- The "operation" field should contain the appropriate kubectl command (e.g., "get", "describe", "delete").
- If no namespace is specified, use "default" for the "namespace" field.
- If no label selectors are present, provide an empty array for "label-selector".
- If no field selectors are present, provide an empty array for "field-selector".
- Do not include response in quotes. The response should be return as text.

Here are two examples to guide you:

Example 1:
User query: "List all pods in the kube-system namespace with the label app=monitoring"
Output:
{
  "operation": "get",
  "resource": "pods",
  "namespace": "kube-system",
  "label-selector": ["app=monitoring"],
  "field-selector": []
}

Example 2:
User query: "Delete the deployment named 'nginx-deployment' in the 'web' namespace"
Output:
{
  "operation": "delete",
  "resource": "deployment",
  "namespace": "web",
  "label-selector": [],
  "field-selector": []
}`
	resp, err := o.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT4o,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: prompt,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: fmt.Sprintf("<user_query>{{%s}}</user_query>", query),
				},
			},
		},
	)

	if err != nil {
		return "", err
	}

	return resp.Choices[0].Message.Content, nil
}

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

func (o *OpenAI) Request(prompt, query string) (string, error) {
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

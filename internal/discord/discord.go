package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	MaxMessageLength = 1800 // Discord's maximum message length
)

// EmbedField represents a field in the embed
type EmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

// Embed represents the structure of an embed message
type Embed struct {
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Color       int          `json:"color"`
	Fields      []EmbedField `json:"fields"`
}

// WebhookMessage represents the JSON payload for a webhook
type WebhookMessage struct {
	Content string  `json:"content,omitempty"`
	Embeds  []Embed `json:"embeds"`
}

// discordServiceImpl implements the DiscordService interface
type discordServiceImpl struct {
	webhookURL string
	bot        DiscordBot
}

// NewDiscordService creates a new instance of DiscordService
func NewDiscordService(webhook, token string) DiscordService {
	return &discordServiceImpl{
		webhookURL: webhook,
		bot:        NewDiscordBot(token),
	}
}

// SendMessage sends a message to the Discord webhook, splitting it into chunks if necessary
func (d *discordServiceImpl) SendMessage(content string) error {
	if len(content) <= MaxMessageLength {
		return d.sendSingleMessage(content)
	}

	chunks := splitMessage(content)
	for i, chunk := range chunks {
		err := d.sendSingleMessage(fmt.Sprintf("Part %d/%d:\n%s", i+1, len(chunks), chunk))
		if err != nil {
			return fmt.Errorf("failed to send message chunk %d: %v", i+1, err)
		}
	}

	return nil
}

// SendWebhookMessage sends a message to a Discord  webhook with an embed
func (d *discordServiceImpl) SendWebhookMessage(webhookURL string, content string, embeds []Embed) error {
	if len(content) <= MaxMessageLength {
		return d.sendSingleMessage(content)
	}

	// Create the webhook message payload
	message := WebhookMessage{
		Content: content,
		Embeds:  embeds,
	}

	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	resp, err := http.Post(d.webhookURL, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to send webhook message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("received non-204 response: %d", resp.StatusCode)
	}

	return nil
}

// sendSingleMessage sends a single message to the Discord webhook
func (d *discordServiceImpl) sendSingleMessage(content string) error {
	message := map[string]string{
		"content": content,
	}

	jsonMessage, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal Discord message: %v", err)
	}

	resp, err := http.Post(d.webhookURL, "application/json", bytes.NewBuffer(jsonMessage))
	if err != nil {
		return fmt.Errorf("failed to send Discord message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Discord API returned non-OK status: %d", resp.StatusCode)
	}

	return nil
}

// splitMessage splits a long message into chunks of MaxMessageLength or less
func splitMessage(content string) []string {
	var chunks []string
	for len(content) > 0 {
		if len(content) <= MaxMessageLength {
			chunks = append(chunks, content)
			break
		}

		chunk := content[:MaxMessageLength]
		lastNewline := bytes.LastIndexByte([]byte(chunk), '\n')
		if lastNewline != -1 {
			chunk = content[:lastNewline]
		}

		chunks = append(chunks, chunk)
		content = content[len(chunk):]
	}
	return chunks
}

// DiscordBot returns the Bot implementation
func (d *discordServiceImpl) Bot() DiscordBot {
	return d.bot
}

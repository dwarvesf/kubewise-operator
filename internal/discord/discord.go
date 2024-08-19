package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// discordServiceImpl implements the DiscordService interface
type discordServiceImpl struct {
	WebhookURL string
}

// NewDiscordService creates a new instance of DiscordService
func NewDiscordService(webhookURL string) DiscordService {
	return &discordServiceImpl{
		WebhookURL: webhookURL,
	}
}

// SendMessage sends a message to the Discord webhook
func (d *discordServiceImpl) SendMessage(content string) error {
	message := map[string]string{
		"content": content,
	}

	jsonMessage, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal Discord message: %v", err)
	}

	resp, err := http.Post(d.WebhookURL, "application/json", bytes.NewBuffer(jsonMessage))
	if err != nil {
		return fmt.Errorf("failed to send Discord message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Discord API returned non-OK status: %d", resp.StatusCode)
	}

	return nil
}
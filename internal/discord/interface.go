package discord

import (
	"github.com/bwmarrin/discordgo"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dwarvesf/kubewise-operator/internal/gpt"
)

type DiscordBot interface {
	Open() error
	Close() error
	AddHandler(handler func(s *discordgo.Session, m *discordgo.MessageCreate))
	SendMessage(channelID string, content string) (*discordgo.Message, error)
	SetGPT(gpt gpt.GPT)
	SetK8sClient(client client.Client)
}

// DiscordService defines the interface for sending messages to Discord
type DiscordService interface {
	SendMessage(content string) error
	SendWebhookMessage(webhookURL string, content string, embeds []Embed) error
	Bot() DiscordBot
}

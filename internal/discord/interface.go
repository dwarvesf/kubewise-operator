package discord

// DiscordService defines the interface for sending messages to Discord
type DiscordService interface {
	SendMessage(content string) error
}
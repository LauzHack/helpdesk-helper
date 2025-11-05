// Package context/context.go
package context

import (
	"lauzhack-bot/botapi"

	"github.com/bwmarrin/discordgo"
)

const FlagEphemeral = 1 << 6

type CommandContext struct {
	Session *discordgo.Session
	Bot     botapi.BotAPI
}

func (c *CommandContext) Reply(ic *discordgo.InteractionCreate, content string) {
	_ = c.Session.InteractionRespond(ic.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   FlagEphemeral,
		},
	})
}

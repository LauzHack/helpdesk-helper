package discord

import (
	"lauzhack-bot/botapi"
	"lauzhack-bot/discord/context"
	"lauzhack-bot/discord/internal/admin"
	"lauzhack-bot/discord/internal/user"

	"github.com/bwmarrin/discordgo"
)

var bot botapi.BotAPI

func Init(b botapi.BotAPI) {
	bot = b
}

func OnInteractionCreate(s *discordgo.Session, ic *discordgo.InteractionCreate) {
	if ic.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if ic.Member == nil || ic.Member.User == nil || ic.Member.User.Bot {
		return
	}

	ctx := &context.CommandContext{Session: s, Bot: bot}
	data := ic.ApplicationCommandData()

	switch data.Name {
	case "helpdesk":
		user.HandleHelpdeskCommand(ctx, ic, data)
	case "admin":
		admin.HandleAdminCommand(ctx, ic, data)
	default:
		ctx.Reply(ic, "Unknown command.")
	}
}

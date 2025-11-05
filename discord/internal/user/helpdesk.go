// Package user
package user

import (
	"fmt"
	"lauzhack-bot/config"
	"lauzhack-bot/discord/context"
	"lauzhack-bot/scheduler"
	"lauzhack-bot/utils"
	"time"

	"github.com/bwmarrin/discordgo"
)

func HandleHelpdeskCommand(ctx *context.CommandContext, ic *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData) {
	if len(data.Options) == 0 {
		ctx.Reply(ic, "No subcommand.")
		return
	}

	switch data.Options[0].Name {
	case "join":
		handleJoin(ctx, ic)
	case "leave":
		handleLeave(ctx, ic)
	case "list":
		handleList(ctx, ic)
	default:
		ctx.Reply(ic, "Unknown subcommand.")
	}
}

func handleJoin(ctx *context.CommandContext, ic *discordgo.InteractionCreate) {
	store := config.GetStore()
	userID := ic.Member.User.ID

	if ctx.Bot.HasRoleNow(userID) {
		ctx.Reply(ic, "You already have the helpdesk role.")
		return
	}

	if err := ctx.Bot.AddRole(userID); err != nil {
		ctx.Reply(ic, "Failed to add role (permissions?): "+err.Error())
		return
	}

	if !utils.Contains(store.Volunteers, userID) {
		store.Volunteers = append(store.Volunteers, userID)
		_ = config.SaveStore()
	}

	ctx.Reply(ic, "You are now part of the helpdesk.")
}

func handleLeave(ctx *context.CommandContext, ic *discordgo.InteractionCreate) {
	userID := ic.Member.User.ID
	store := config.GetStore()

	if utils.Contains(store.Volunteers, userID) {
		store.Volunteers = utils.Remove(store.Volunteers, userID)
		_ = config.SaveStore()
	}

	if !scheduler.IsUserScheduledNow(userID) {
		if err := ctx.Bot.RemoveRole(userID); err != nil {
			ctx.Reply(ic, "Failed to remove role (permissions?): "+err.Error())
			return
		}
	}

	ctx.Reply(ic, "You have left the helpdesk.")
}

func handleList(ctx *context.CommandContext, ic *discordgo.InteractionCreate) {
	now := time.Now()
	cur, next := scheduler.CurrentAndNextShift(now)
	store := config.GetStore()

	embed := &discordgo.MessageEmbed{
		Title:       "Helpdesk Schedule",
		Description: "Current and upcoming shifts",
		Color:       0x00BFFF,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	// Current shift
	if cur.Start != 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Now",
			Value:  fmt.Sprintf("%s\n<t:%d:R> → <t:%d:R>", utils.MentionUsers(cur.UserIDs), cur.Start, cur.End),
			Inline: false,
		})
	} else {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Now",
			Value:  "(no active shift)",
			Inline: false,
		})
	}

	// Next shift
	if next.Start != 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Next",
			Value:  fmt.Sprintf("%s\n<t:%d:R> → <t:%d:R>", utils.MentionUsers(next.UserIDs), next.Start, next.End),
			Inline: false,
		})
	}

	// Volunteers
	if len(store.Volunteers) > 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Volunteers",
			Value:  utils.MentionUsers(store.Volunteers),
			Inline: false,
		})
	}

	if err := ctx.Session.InteractionRespond(ic.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  1 << 6, // ephemeral
		},
	}); err != nil {
		ctx.Reply(ic, fmt.Sprintf("Failed to send list: %v", err))
	}
}

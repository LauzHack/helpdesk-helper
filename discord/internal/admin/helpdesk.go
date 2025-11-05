// Package admin
package admin

import (
	"fmt"
	"lauzhack-bot/config"
	"lauzhack-bot/discord/context"
	"lauzhack-bot/scheduler"
	"lauzhack-bot/utils"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func HandleAdminCommand(ctx *context.CommandContext, ic *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData) {
	if len(data.Options) == 0 {
		ctx.Reply(ic, "Missing group.")
		return
	}
	group := data.Options[0]
	if group.Name != "helpdesk" || len(group.Options) == 0 {
		ctx.Reply(ic, "Unknown admin group or subcommand.")
		return
	}

	sub := group.Options[0]
	switch sub.Name {
	case "add":
		handleAdd(ctx, ic, sub)
	case "remove":
		handleRemove(ctx, ic, sub)
	case "sync":
		handleSync(ctx, ic)
	default:
		ctx.Reply(ic, "Unknown admin subcommand.")
	}
}

func handleAdd(ctx *context.CommandContext, ic *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	user := sub.Options[0].UserValue(ctx.Session)
	if user == nil {
		ctx.Reply(ic, "User not found.")
		return
	}
	if err := ctx.Bot.AddRole(user.ID); err != nil {
		ctx.Reply(ic, "Failed to add role: "+err.Error())
		return
	}
	ctx.Reply(ic, fmt.Sprintf("Added %s to helpdesk role.", utils.MentionUsers([]string{user.ID})))
}

func handleRemove(ctx *context.CommandContext, ic *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	user := sub.Options[0].UserValue(ctx.Session)
	if user == nil {
		ctx.Reply(ic, "User not found.")
		return
	}
	if err := ctx.Bot.RemoveRole(user.ID); err != nil {
		ctx.Reply(ic, "Failed to remove role: "+err.Error())
		return
	}
	if utils.Contains(config.Store.Volunteers, user.ID) {
		config.Store.Volunteers = utils.Remove(config.Store.Volunteers, user.ID)
		_ = config.SaveStore()
	}
	ctx.Reply(ic, fmt.Sprintf("Removed %s from helpdesk role.", utils.MentionUsers([]string{user.ID})))
}

func handleSync(ctx *context.CommandContext, ic *discordgo.InteractionCreate) {
	now := time.Now().In(config.Loc)
	applied, errs := scheduler.ApplyShiftState(now)
	msg := "Sync done. " + applied
	if len(errs) > 0 {
		msg += "\nError:\n- " + strings.Join(errs, "\n- ")
	}
	ctx.Reply(ic, msg)
}

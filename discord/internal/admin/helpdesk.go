// Package admin
package admin

import (
	"fmt"
	"strings"
	"time"

	"lauzhack-bot/discord/context"
	"lauzhack-bot/scheduler"
	"lauzhack-bot/store"
	"lauzhack-bot/utils"

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

	if err := store.AddVolunteer(user.ID); err != nil {
		ctx.Reply(ic, "Failed to add volunteer: "+err.Error())
		return
	}

	ctx.Reply(ic, fmt.Sprintf("✅ Added %s to helpdesk.", utils.MentionUsers([]string{user.ID})))
}

func handleRemove(ctx *context.CommandContext, ic *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	user := sub.Options[0].UserValue(ctx.Session)
	if user == nil {
		ctx.Reply(ic, "User not found.")
		return
	}

	// Remove Discord role
	if err := ctx.Bot.RemoveRole(user.ID); err != nil {
		ctx.Reply(ic, "Failed to remove role: "+err.Error())
		return
	}

	// Remove from volunteer list (if present)
	if err := store.RemoveVolunteer(user.ID); err != nil {
		ctx.Reply(ic, "Failed to update store: "+err.Error())
		return
	}

	ctx.Reply(ic, fmt.Sprintf("🗑️ Removed %s from helpdesk.", utils.MentionUsers([]string{user.ID})))
}

func handleSync(ctx *context.CommandContext, ic *discordgo.InteractionCreate) {
	now := time.Now()
	summary, errs := scheduler.ApplyShiftState(now)

	var b strings.Builder
	b.WriteString("**Helpdesk sync complete.**\n\n")
	b.WriteString(summary)

	if len(errs) > 0 {
		b.WriteString("\n⚠️ Errors:\n- " + strings.Join(errs, "\n- "))
	}

	ctx.Reply(ic, b.String())
}

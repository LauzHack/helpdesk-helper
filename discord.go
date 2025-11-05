package main

// TODO: Make this into a package

import (
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

const FlagEphemeral = 64

func onInteractionCreate(s *discordgo.Session, ic *discordgo.InteractionCreate) {
	if ic.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if ic.Member == nil || ic.Member.User == nil || ic.Member.User.Bot {
		return
	}

	data := ic.ApplicationCommandData()
	switch data.Name {
	case "helpdesk":
		handleHelpdeskCommand(s, ic, data)
	case "admin":
		handleAdminCommand(s, ic, data)
	default:
		reply(s, ic, "Unknown command.")
	}
}

// /helpdesk
func handleHelpdeskCommand(s *discordgo.Session, ic *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData) {
	if len(data.Options) == 0 {
		reply(s, ic, "No subcommand.")
		return
	}
	switch data.Options[0].Name {
	case "join":
		handleJoin(s, ic)
	case "leave":
		handleLeave(s, ic)
	case "list":
		handleList(s, ic)
	default:
		reply(s, ic, "Unknown subcommand.")
	}
}

// /admin helpdesk
func handleAdminCommand(s *discordgo.Session, ic *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData) {
	if !userHasRole(ic.Member, cfg.OrganizerRoleID) && !isAdmin(ic) {
		reply(s, ic, "You do not have permission to run this command.")
		return
	}
	if len(data.Options) == 0 {
		reply(s, ic, "Missing group.")
		return
	}

	group := data.Options[0]
	if group.Name != "helpdesk" || len(group.Options) == 0 {
		reply(s, ic, "Unknown admin group or subcommand.")
		return
	}

	sub := group.Options[0]
	switch sub.Name {
	case "add":
		handleAdminAdd(s, ic, sub)
	case "remove":
		handleAdminRemove(s, ic, sub)
	case "sync":
		handleAdminSync(s, ic)
	default:
		reply(s, ic, "Unknown admin subcommand.")
	}
}

func handleJoin(s *discordgo.Session, ic *discordgo.InteractionCreate) {
	userID := ic.Member.User.ID

	if hasRoleNow(userID) {
		reply(s, ic, "You already have the helpdesk role.")
		return
	}

	if err := addRole(userID); err != nil {
		reply(s, ic, "Failed to add role (permissions?): "+err.Error())
		return
	}

	if !contains(store.Volunteers, userID) {
		store.Volunteers = append(store.Volunteers, userID)
		_ = saveStore()
	}

	reply(s, ic, "You are on helpdesk.")
}

func handleLeave(s *discordgo.Session, ic *discordgo.InteractionCreate) {
	userID := ic.Member.User.ID
	if contains(store.Volunteers, userID) {
		store.Volunteers = remove(store.Volunteers, userID)
		_ = saveStore()
	}

	if !isUserScheduledNow(userID) {
		if err := removeRole(userID); err != nil {
			reply(s, ic, "Failed to remove role (permissions?): "+err.Error())
			return
		}
	}

	reply(s, ic, "You have left helpdesk.")
}

func handleList(s *discordgo.Session, ic *discordgo.InteractionCreate) {
	now := time.Now().In(loc)
	cur, next := currentAndNextShift(now)

	embed := &discordgo.MessageEmbed{
		Title:       "Helpdesk Schedule",
		Description: "Current and upcoming shifts",
		Color:       0x00BFFF,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	if cur.Start != 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Now",
			Value:  fmt.Sprintf("%s\n<t:%d:R> → <t:%d:R>", mentionUsers(cur.UserIDs), cur.Start, cur.End),
			Inline: false,
		})
	} else {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Now",
			Value:  "(no active shift)",
			Inline: false,
		})
	}

	if next.Start != 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Next",
			Value:  fmt.Sprintf("%s\n<t:%d:R> → <t:%d:R>", mentionUsers(next.UserIDs), next.Start, next.End),
			Inline: false,
		})
	}

	if len(store.Volunteers) > 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Volunteers",
			Value:  mentionUsers(store.Volunteers),
			Inline: false,
		})
	}

	if err := s.InteractionRespond(ic.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  FlagEphemeral,
		},
	}); err != nil {
		log.Printf("interaction respond: %v", err)
	}
}

func handleAdminAdd(s *discordgo.Session, ic *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	user := sub.Options[0].UserValue(s)
	if user == nil {
		reply(s, ic, "User not found.")
		return
	}

	if err := addRole(user.ID); err != nil {
		reply(s, ic, "Failed to add role: "+err.Error())
		return
	}
	reply(s, ic, fmt.Sprintf("Added %s to helpdesk role.", mentionUsers([]string{user.ID})))
}

func handleAdminRemove(s *discordgo.Session, ic *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	user := sub.Options[0].UserValue(s)
	if user == nil {
		reply(s, ic, "User not found.")
		return
	}

	if err := removeRole(user.ID); err != nil {
		reply(s, ic, "Failed to remove role: "+err.Error())
		return
	}
	if contains(store.Volunteers, user.ID) {
		store.Volunteers = remove(store.Volunteers, user.ID)
		_ = saveStore()
	}
	reply(s, ic, fmt.Sprintf("Removed %s from helpdesk role.", mentionUsers([]string{user.ID})))
}

func handleAdminSync(s *discordgo.Session, ic *discordgo.InteractionCreate) {
	now := time.Now().In(loc)
	applied, errs := applyShiftState(now)
	msg := "Sync done. " + applied
	if len(errs) > 0 {
		msg += "\nError:\n- " + strings.Join(errs, "\n- ")
	}
	reply(s, ic, msg)
}

func reply(s *discordgo.Session, ic *discordgo.InteractionCreate, content string) {
	if err := s.InteractionRespond(ic.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Flags: FlagEphemeral},
	}); err != nil {
		log.Printf("interaction respond: %v", err)
	}
}

func isAdmin(ic *discordgo.InteractionCreate) bool {
	const PermAdmin = 0x00000008
	return ic.Member != nil && (ic.Member.Permissions&PermAdmin) == PermAdmin
}

func userHasRole(m *discordgo.Member, roleID string) bool {
	return m != nil && slices.Contains(m.Roles, roleID)
}

// Role ops
func addRole(userID string) error {
	for i := range 3 {
		if err := dg.GuildMemberRoleAdd(cfg.GuildID, userID, cfg.RoleID); err == nil {
			return nil
		}
		time.Sleep(time.Second * time.Duration(i+1))
	}
	return fmt.Errorf("addRole %s failed after retries", userID)
}

func removeRole(userID string) error {
	for i := range 3 {
		if err := dg.GuildMemberRoleRemove(cfg.GuildID, userID, cfg.RoleID); err == nil {
			return nil
		}
		time.Sleep(time.Second * time.Duration(i+1))
	}
	return fmt.Errorf("removeRole %s failed after retries", userID)
}

func hasRoleNow(userID string) bool {
	m, err := dg.GuildMember(cfg.GuildID, userID)
	if err != nil {
		return false
	}
	return slices.Contains(m.Roles, cfg.RoleID)
}

func getAllMembers(guildID string) ([]*discordgo.Member, error) {
	var members []*discordgo.Member
	after := ""
	for {
		chunk, err := dg.GuildMembers(guildID, after, 1000)
		if err != nil {
			return members, err
		}
		if len(chunk) == 0 {
			break
		}
		members = append(members, chunk...)
		after = chunk[len(chunk)-1].User.ID
	}
	return members, nil
}

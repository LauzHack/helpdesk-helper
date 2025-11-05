// Package discord/discord.go
package discord

import (
	"fmt"
	"lauzhack-bot/config"
	"slices"
	"time"

	"github.com/bwmarrin/discordgo"
)

type DiscordBot struct {
	Session *discordgo.Session
}

func New(session *discordgo.Session) *DiscordBot {
	return &DiscordBot{Session: session}
}

// Role ops

func (d *DiscordBot) AddRole(userID string) error {
	for i := range 3 {
		if err := d.Session.GuildMemberRoleAdd(config.Cfg.GuildID, userID, config.Cfg.RoleID); err == nil {
			return nil
		}
		time.Sleep(time.Second * time.Duration(i+1))
	}
	return fmt.Errorf("addRole %s failed after retries", userID)
}

func (d *DiscordBot) RemoveRole(userID string) error {
	for i := range 3 {
		if err := d.Session.GuildMemberRoleRemove(config.Cfg.GuildID, userID, config.Cfg.RoleID); err == nil {
			return nil
		}
		time.Sleep(time.Second * time.Duration(i+1))
	}
	return fmt.Errorf("removeRole %s failed after retries", userID)
}

func (d *DiscordBot) HasRoleNow(userID string) bool {
	m, err := d.Session.GuildMember(config.Cfg.GuildID, userID)
	if err != nil {
		return false
	}
	return slices.Contains(m.Roles, config.Cfg.RoleID)
}

func (d *DiscordBot) GetAllMembers(guildID string) ([]*discordgo.Member, error) {
	var members []*discordgo.Member
	after := ""
	for {
		chunk, err := d.Session.GuildMembers(guildID, after, 1000)
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

func (d *DiscordBot) SendReminder(userID string, start time.Time) error {
	msg := fmt.Sprintf(
		"<@%s> Reminder: your helpdesk shift starts <t:%d:R>. Please head to the helpdesk desk during that timeframe.",
		userID, start.Unix(),
	)
	_, err := d.Session.ChannelMessageSend(config.Cfg.LogChannelID, msg)
	return err
}

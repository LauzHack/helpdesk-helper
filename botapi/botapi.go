// Package botapi/botapi.go
package botapi

import (
	"time"

	"github.com/bwmarrin/discordgo"
)

type BotAPI interface {
	AddRole(userID string) error
	RemoveRole(userID string) error
	HasRoleNow(userID string) bool
	GetAllMembers(guildID string) ([]*discordgo.Member, error)
	SendReminder(userID string, start time.Time) error
}

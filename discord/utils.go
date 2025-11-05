package discord

import (
	"log"
	"slices"

	"github.com/bwmarrin/discordgo"
)

func Reply(s *discordgo.Session, ic *discordgo.InteractionCreate, content string) {
	if err := s.InteractionRespond(ic.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Flags: 64},
	}); err != nil {
		log.Printf("interaction respond: %v", err)
	}
}

func IsAdmin(ic *discordgo.InteractionCreate) bool {
	const PermAdmin = 0x00000008
	return ic.Member != nil && (ic.Member.Permissions&PermAdmin) == PermAdmin
}

func UserHasRole(m *discordgo.Member, roleID string) bool {
	return m != nil && slices.Contains(m.Roles, roleID)
}

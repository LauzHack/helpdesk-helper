// Package types/types.go
package types

type Config struct {
	Token           string `json:"token"`
	GuildID         string `json:"guild_id"`
	RoleID          string `json:"role_id"`
	OrganizerRoleID string `json:"organizer_role_id"`
	LogChannelID    string `json:"log_channel_id"`
	DataFile        string `json:"data_file"`
	AdminToken      string `json:"admin_token"`
}

// Package data/types.go
package types

type Config struct {
	Token           string `json:"token"`
	GuildID         string `json:"guild_id"`
	RoleID          string `json:"role_id"`
	OrganizerRoleID string `json:"organizer_role_id"`
	LogChannelID    string `json:"log_channel_id"`
	DataFile        string `json:"data_file"`
}

type Shift struct {
	// Unix timestamps
	Start   int64    `json:"start"`
	End     int64    `json:"end"`
	UserIDs []string `json:"user_ids"`
}

type Store struct {
	Volunteers []string `json:"volunteers"`
	Schedule   []Shift  `json:"schedule"`
}

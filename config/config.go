// Package config
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"lauzhack-bot/types"
)

var Cfg types.Config

func LoadConfig(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &Cfg); err != nil {
		return err
	}
	if Cfg.Token == "" || Cfg.GuildID == "" || Cfg.RoleID == "" || Cfg.DataFile == "" ||
		Cfg.OrganizerRoleID == "" || Cfg.LogChannelID == "" || Cfg.AdminToken == "" {
		return errors.New("missing required fields in config file")
	}
	if !filepath.IsAbs(Cfg.DataFile) {
		Cfg.DataFile = filepath.Join(filepath.Dir(path), Cfg.DataFile)
	}
	return nil
}

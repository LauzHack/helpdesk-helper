// Package config/config.go
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"lauzhack-bot/data"
)

var (
	Cfg   data.Config
	Store data.Store
	Loc   *time.Location
)

func LoadConfig(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &Cfg); err != nil {
		return err
	}
	if Cfg.Token == "" || Cfg.GuildID == "" || Cfg.RoleID == "" || Cfg.Timezone == "" || Cfg.DataFile == "" || Cfg.OrganizerRoleID == "" {
		return errors.New("missing required fields in config file")
	}
	if !filepath.IsAbs(Cfg.DataFile) {
		Cfg.DataFile = filepath.Join(filepath.Dir(path), Cfg.DataFile)
	}
	return nil
}

func LoadStore() error {
	if _, err := os.Stat(Cfg.DataFile); err != nil {
		if os.IsNotExist(err) {
			Store = data.Store{Volunteers: []string{}, Schedule: []data.Shift{}}
			return SaveStore()
		}
		return err
	}
	b, err := os.ReadFile(Cfg.DataFile)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &Store)
}

func SaveStore() error {
	tmp := Cfg.DataFile + ".tmp"
	b, _ := json.MarshalIndent(Store, "", "  ")
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, Cfg.DataFile)
}

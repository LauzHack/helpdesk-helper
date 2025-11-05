package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"lauzhack-bot/data"
)

func loadConfig(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return err
	}
	if cfg.Token == "" || cfg.GuildID == "" || cfg.RoleID == "" || cfg.Timezone == "" || cfg.DataFile == "" || cfg.OrganizerRoleID == "" {
		return errors.New("missing required fields in config file")
	}
	if !filepath.IsAbs(cfg.DataFile) {
		cfg.DataFile = filepath.Join(filepath.Dir(path), cfg.DataFile)
	}
	return nil
}

func loadStore() error {
	if _, err := os.Stat(cfg.DataFile); err != nil {
		if os.IsNotExist(err) {
			store = data.Store{Volunteers: []string{}, Schedule: []data.Shift{}}
			return saveStore()
		}
		return err
	}
	b, err := os.ReadFile(cfg.DataFile)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &store)
}

func saveStore() error {
	tmp := cfg.DataFile + ".tmp"
	b, _ := json.MarshalIndent(store, "", "  ")
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
	return os.Rename(tmp, cfg.DataFile)
}

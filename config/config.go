// Package config
package config

import (
	"encoding/json"
	"errors"
	"lauzhack-bot/data"
	"os"
	"path/filepath"
	"sync"
)

var (
	Cfg     data.Config
	storeMu sync.RWMutex
	store   data.Store
)

func LoadConfig(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &Cfg); err != nil {
		return err
	}
	if Cfg.Token == "" || Cfg.GuildID == "" || Cfg.RoleID == "" || Cfg.DataFile == "" ||
		Cfg.OrganizerRoleID == "" || Cfg.LogChannelID == "" {
		return errors.New("missing required fields in config file")
	}
	if !filepath.IsAbs(Cfg.DataFile) {
		Cfg.DataFile = filepath.Join(filepath.Dir(path), Cfg.DataFile)
	}
	return nil
}

func LoadStore() error {
	storeMu.Lock()
	defer storeMu.Unlock()

	if _, err := os.Stat(Cfg.DataFile); err != nil {
		if os.IsNotExist(err) {
			store = data.Store{Volunteers: []string{}, Schedule: []data.Shift{}}
			return saveStoreLocked()
		}
		return err
	}
	b, err := os.ReadFile(Cfg.DataFile)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &store)
}

func SaveStore() error {
	storeMu.RLock()
	defer storeMu.RUnlock()
	return saveStoreLocked()
}

// Internal unsafe variant — must hold lock
func saveStoreLocked() error {
	tmp := Cfg.DataFile + ".tmp"
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
	return os.Rename(tmp, Cfg.DataFile)
}

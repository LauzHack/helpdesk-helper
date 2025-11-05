package config

import "lauzhack-bot/data"

// GetStore returns a copy of the current store (safe read)
func GetStore() data.Store {
	storeMu.RLock()
	defer storeMu.RUnlock()
	return store
}

// UpdateStore applies a mutation safely
func UpdateStore(fn func(s *data.Store)) {
	storeMu.Lock()
	defer storeMu.Unlock()
	fn(&store)
	_ = saveStoreLocked()
}

// ReplaceStore overwrites the store (rarely used)
func ReplaceStore(newStore data.Store) {
	storeMu.Lock()
	defer storeMu.Unlock()
	store = newStore
	_ = saveStoreLocked()
}

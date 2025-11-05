package config

import (
	"lauzhack-bot/types"
)

// GetStore returns a copy of the current store (safe read)
func GetStore() types.Store {
	storeMu.RLock()
	defer storeMu.RUnlock()
	return store
}

// UpdateStore applies a mutation safely
func UpdateStore(fn func(s *types.Store)) {
	storeMu.Lock()
	defer storeMu.Unlock()
	fn(&store)
	_ = saveStoreLocked()
}

// ReplaceStore overwrites the store (rarely used)
func ReplaceStore(newStore types.Store) {
	storeMu.Lock()
	defer storeMu.Unlock()
	store = newStore
	_ = saveStoreLocked()
}

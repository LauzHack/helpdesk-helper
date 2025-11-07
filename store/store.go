// Package store
package store

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"sync"
	"time"
)

type Store struct {
	Volunteers []string `json:"volunteers"`
	Schedule   []Shift  `json:"schedule"`
}

type Shift struct {
	Start   int64    `json:"start"`
	End     int64    `json:"end"`
	UserIDs []string `json:"user_ids"`
}

var (
	data Store
	mu   sync.RWMutex
	path string
)

func Init(filepath string) error {
	path = filepath
	mu.Lock()
	defer mu.Unlock()

	b, err := os.ReadFile(filepath)
	if errors.Is(err, os.ErrNotExist) {
		data = Store{}
		return Save()
	}
	if err != nil {
		return err
	}

	return json.Unmarshal(b, &data)
}

func Save() error {
	tmp := path + ".tmp"
	b, err := json.MarshalIndent(&data, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}

func AddVolunteer(id string) error {
	mu.Lock()
	defer mu.Unlock()

	if slices.Contains(data.Volunteers, id) {
		return nil
	}

	data.Volunteers = append(data.Volunteers, id)
	return Save()
}

func RemoveVolunteer(id string) error {
	mu.Lock()
	defer mu.Unlock()
	out := data.Volunteers[:0]

	for _, v := range data.Volunteers {
		if v != id {
			out = append(out, v)
		}
	}

	data.Volunteers = out
	return Save()
}

func HasVolunteer(id string) bool {
	mu.Lock()
	defer mu.Unlock()

	return slices.Contains(data.Volunteers, id)
}

func ListVolunteers() []string {
	mu.RLock()
	defer mu.RUnlock()

	out := make([]string, len(data.Volunteers))
	copy(out, data.Volunteers)
	return out
}

func NextVolunteer() (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if len(data.Volunteers) == 0 {
		return "", errors.New("no volunteers")
	}

	id := data.Volunteers[0]
	data.Volunteers = data.Volunteers[1:]
	if err := Save(); err != nil {
		return "", err
	}

	return id, nil
}

func AddShift(s Shift) error {
	mu.Lock()
	defer mu.Unlock()
	data.Schedule = append(data.Schedule, s)
	return Save()
}

func ListShifts() []Shift {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Shift, len(data.Schedule))
	copy(out, data.Schedule)
	return out
}

func UpdateShift(i int, s Shift) error {
	mu.Lock()
	defer mu.Unlock()
	if i < 0 || i >= len(data.Schedule) {
		return errors.New("index out of range")
	}
	data.Schedule[i] = s
	return Save()
}

func RemoveShift(start int64) error {
	mu.Lock()
	defer mu.Unlock()
	out := data.Schedule[:0]
	for _, sh := range data.Schedule {
		if sh.Start != start {
			out = append(out, sh)
		}
	}
	data.Schedule = out
	return Save()
}

func ReplaceAllShifts(shifts []Shift) {
	mu.Lock()
	defer mu.Unlock()
	data.Schedule = append([]Shift(nil), shifts...) // copy
	_ = Save()
}

func NextShift(now int64) (*Shift, error) {
	mu.RLock()
	defer mu.RUnlock()
	var next *Shift
	for i := range data.Schedule {
		s := &data.Schedule[i]
		if s.Start > now && (next == nil || s.Start < next.Start) {
			next = s
		}
	}
	if next == nil {
		return nil, errors.New("no upcoming shifts")
	}
	cp := *next
	return &cp, nil
}

func CurrentAndNextShift(now time.Time) (Shift, Shift) {
	mu.RLock()
	defer mu.RUnlock()

	var cur, next Shift
	var nextStart int64

	for _, sh := range data.Schedule {
		start := time.Unix(sh.Start, 0)
		end := time.Unix(sh.End, 0)

		// Skip invalid ranges
		if sh.End <= sh.Start {
			continue
		}

		// Current shift: now between start and end
		if now.After(start) && now.Before(end) && cur.Start == 0 {
			cur = sh
		}

		// Next shift: earliest start time after now
		if sh.Start > now.Unix() && (next.Start == 0 || sh.Start < nextStart) {
			next = sh
			nextStart = sh.Start
		}
	}

	return cur, next
}

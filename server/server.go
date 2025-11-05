// Package server/server.go
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"lauzhack-bot/config"
	"lauzhack-bot/data"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

var discordSession *discordgo.Session

func Init(s *discordgo.Session) {
	discordSession = s
}

func Start(_ string, addr string) {
	mux := http.NewServeMux()

	// Static UI
	mux.Handle("/", http.FileServer(http.Dir("server/ui")))

	// Schedule CRUD
	mux.HandleFunc("/api/schedule", handleSchedule)         // GET full, POST replace
	mux.HandleFunc("/api/shift/", handleShiftByIndex)       // PUT/DELETE /api/shift/{idx}
	mux.HandleFunc("/api/shift", handleShiftCreate)         // POST add
	mux.HandleFunc("/api/volunteers", handleVolunteers)     // GET/POST
	mux.HandleFunc("/api/volunteers/", handleVolunteerByID) // DELETE /api/volunteers/{id}
	mux.HandleFunc("/api/organizers", handleOrganizers)     // GET /api/organizers
	mux.HandleFunc("/api/status", handleStatus)             // optional: current/next preview

	go func() {
		if err := http.ListenAndServe(addr, cors(mux)); err != nil {
			panic(err)
		}
	}()
}

// Handlers

func handleSchedule(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, config.GetStore())
	case http.MethodPost:
		var updated data.Store
		if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if err := validateStore(updated); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		config.ReplaceStore(updated)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleShiftCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var s data.Shift
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := validateShift(s); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var newIdx int
	config.UpdateStore(func(st *data.Store) {
		st.Schedule = append(st.Schedule, s)
		newIdx = len(st.Schedule) - 1
	})
	writeJSON(w, http.StatusCreated, map[string]int{"index": newIdx})
}

func handleShiftByIndex(w http.ResponseWriter, r *http.Request) {
	idxStr := path.Base(r.URL.Path)
	i, err := strconv.Atoi(idxStr)
	if err != nil || i < 0 {
		http.Error(w, "invalid index", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var s data.Shift
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if err := validateShift(s); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ok := configSafeUpdateIndex(i, func(st *data.Store) { st.Schedule[i] = s })
		if !ok {
			http.Error(w, "index out of range", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	case http.MethodDelete:
		ok := configSafeUpdateIndex(i, func(st *data.Store) {
			st.Schedule = append(st.Schedule[:i], st.Schedule[i+1:]...)
		})
		if !ok {
			http.Error(w, "index out of range", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleVolunteers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, config.GetStore().Volunteers)
	case http.MethodPost:
		var body struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.UserID) == "" {
			http.Error(w, "invalid user_id", http.StatusBadRequest)
			return
		}
		config.UpdateStore(func(st *data.Store) {
			if !contains(st.Volunteers, body.UserID) {
				st.Volunteers = append(st.Volunteers, body.UserID)
			}
		})
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleVolunteerByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := path.Base(r.URL.Path)
	if strings.TrimSpace(id) == "" {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	config.UpdateStore(func(st *data.Store) {
		out := st.Volunteers[:0]
		for _, v := range st.Volunteers {
			if v != id {
				out = append(out, v)
			}
		}
		st.Volunteers = out
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	cur, next := currentAndNext(config.GetStore(), now)
	writeJSON(w, http.StatusOK, map[string]any{
		"now":  now.Unix(),
		"cur":  cur,
		"next": next,
	})
}

// GET /api/organizers
func handleOrganizers(w http.ResponseWriter, r *http.Request) {
	if discordSession == nil {
		http.Error(w, "discord session not initialized", http.StatusServiceUnavailable)
		return
	}

	guildID := config.Cfg.GuildID
	roleID := config.Cfg.OrganizerRoleID

	type user struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	var (
		users   []user
		after   string
		batch   []*discordgo.Member
		err     error
		fetched int
	)

	for {
		batch, err = discordSession.GuildMembers(guildID, after, 1000)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if len(batch) == 0 {
			break
		}
		for _, m := range batch {
			if slices.Contains(m.Roles, roleID) {
				name := m.User.Username
				if m.Nick != "" {
					name = fmt.Sprintf("%s (%s)", m.Nick, m.User.Username)
				}
				users = append(users, user{ID: m.User.ID, Name: name})
			}
		}
		fetched += len(batch)
		if len(batch) < 1000 {
			break
		}
		after = batch[len(batch)-1].User.ID
	}

	writeJSON(w, http.StatusOK, users)
}

// Helpers / validation

func configSafeUpdateIndex(i int, fn func(st *data.Store)) bool {
	ok := true
	config.UpdateStore(func(st *data.Store) {
		if i < 0 || i >= len(st.Schedule) {
			ok = false
			return
		}
		fn(st)
	})
	return ok
}

func validateStore(st data.Store) error {
	for _, s := range st.Schedule {
		if err := validateShift(s); err != nil {
			return err
		}
	}
	return nil
}

func validateShift(s data.Shift) error {
	if s.Start <= 0 || s.End <= 0 {
		return errors.New("start/end must be positive unix seconds")
	}
	if s.End <= s.Start {
		return fmt.Errorf("end (%d) must be > start (%d)", s.End, s.Start)
	}
	for _, u := range s.UserIDs {
		if strings.TrimSpace(u) == "" {
			return errors.New("user_ids contains empty string")
		}
	}
	return nil
}

func currentAndNext(st data.Store, now time.Time) (data.Shift, data.Shift) {
	var cur, next data.Shift
	for _, sh := range st.Schedule {
		ps := time.Unix(sh.Start, 0)
		pe := time.Unix(sh.End, 0)
		if (now.Equal(ps) || (now.After(ps) && now.Before(pe))) && cur.Start == 0 {
			cur = sh
		}
		if ps.After(now) && (next.Start == 0 || ps.Before(time.Unix(next.Start, 0))) {
			next = sh
		}
	}
	return cur, next
}

func contains(arr []string, v string) bool {
	return slices.Contains(arr, v)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// Allow local file:// previews or future cross-origin admin UIs if needed.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

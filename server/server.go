// Package server
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"lauzhack-bot/config"
	"lauzhack-bot/store"

	"github.com/bwmarrin/discordgo"
)

var discordSession *discordgo.Session

func Init(s *discordgo.Session) {
	discordSession = s
}

func Start(ctx context.Context, addr string) {
	mux := http.NewServeMux()

	// Static UI
	mux.Handle("/", http.FileServer(http.Dir("server/ui")))

	// API endpoints
	mux.Handle("/api/schedule", requireAuth(http.HandlerFunc(handleSchedule)))
	mux.Handle("/api/shift/", requireAuth(http.HandlerFunc(handleShiftByIndex)))
	mux.Handle("/api/shift", requireAuth(http.HandlerFunc(handleShiftCreate)))
	mux.Handle("/api/volunteers", requireAuth(http.HandlerFunc(handleVolunteers)))
	mux.Handle("/api/volunteers/", requireAuth(http.HandlerFunc(handleVolunteerByID)))
	mux.Handle("/api/organizers", requireAuth(http.HandlerFunc(handleOrganizers)))
	mux.HandleFunc("/api/status", handleStatus)

	mux.HandleFunc("/api/login", handleLogin)
	mux.HandleFunc("/api/logout", handleLogout)

	srv := &http.Server{
		Addr:    addr,
		Handler: cors(mux),
	}

	go func() {
		log.Printf("server listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	log.Println("server shutting down...")

	// Graceful shutdown with 5s timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
	log.Println("server stopped cleanly")
}

// Handlers

// GET: full schedule, POST: replace
func handleSchedule(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, store.ListShifts())

	case http.MethodPost:
		var newShifts []store.Shift
		if err := json.NewDecoder(r.Body).Decode(&newShifts); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		for _, s := range newShifts {
			if err := validateShift(s); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		// Replace all shifts atomically
		store.ReplaceAllShifts(newShifts)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// POST /api/shift
func handleShiftCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var s store.Shift
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := validateShift(s); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := store.AddShift(s); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

// PUT/DELETE /api/shift/{idx}
func handleShiftByIndex(w http.ResponseWriter, r *http.Request) {
	idxStr := path.Base(r.URL.Path)
	i, err := strconv.Atoi(idxStr)
	if err != nil || i < 0 {
		http.Error(w, "invalid index", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var s store.Shift
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if err := validateShift(s); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := store.UpdateShift(i, s); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	case http.MethodDelete:
		shifts := store.ListShifts()
		if i >= len(shifts) {
			http.Error(w, "index out of range", http.StatusNotFound)
			return
		}
		if err := store.RemoveShift(shifts[i].Start); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// GET/POST /api/volunteers
func handleVolunteers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, store.ListVolunteers())

	case http.MethodPost:
		var body struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.UserID) == "" {
			http.Error(w, "invalid user_id", http.StatusBadRequest)
			return
		}
		if err := store.AddVolunteer(body.UserID); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// DELETE /api/volunteers/{id}
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
	if err := store.RemoveVolunteer(id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /api/status
func handleStatus(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	cur, next := store.CurrentAndNextShift(now)
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
		users []user
		after string
	)

	for {
		members, err := discordSession.GuildMembers(guildID, after, 1000)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if len(members) == 0 {
			break
		}
		for _, m := range members {
			if slices.Contains(m.Roles, roleID) {
				name := m.User.Username
				if m.Nick != "" {
					name = fmt.Sprintf("%s (%s)", m.Nick, m.User.Username)
				}
				users = append(users, user{ID: m.User.ID, Name: name})
			}
		}
		if len(members) < 1000 {
			break
		}
		after = members[len(members)-1].User.ID
	}

	writeJSON(w, http.StatusOK, users)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct{ Token string }
	_ = json.NewDecoder(r.Body).Decode(&body)

	if body.Token != config.Cfg.AdminToken {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	sessionVal := randomToken()
	signed := sign(sessionVal)

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    signed,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	csrfToken := randomToken()
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf",
		Value:    csrfToken,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name:   "csrf",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// Helpers

func validateShift(s store.Shift) error {
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

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

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

func requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check session cookie
		c, err := r.Cookie("session")
		if err != nil || !verifySigned(c.Value) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// CSRF protection for non-GET
		if r.Method != http.MethodGet {
			csrfHeader := r.Header.Get("X-CSRF-Token")
			csrfCookie, err := r.Cookie("csrf")
			if err != nil || csrfHeader == "" || csrfCookie.Value != csrfHeader {
				http.Error(w, "csrf violation", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

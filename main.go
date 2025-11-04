package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

type Config struct {
	Token    string `json:"token"`
	GuildID  string `json:"guild_id"`
	RoleID   string `json:"role_id"`
	Timezone string `json:"timezone"`
	DataFile string `json:"data_file"`
}

type Shift struct {
	// RFC3339 timestamps
	Start   string   `json:"start"`
	End     string   `json:"end"`
	UserIDs []string `json:"user_ids"`
}

type Store struct {
	Volunteers []string `json:"volunteers"`
	Schedule   []Shift  `json:"schedule"`
}

var (
	cfg          Config
	store        Store
	loc          *time.Location
	dg           *discordgo.Session
	remindedKey  = make(map[string]bool)
	activatedKey = make(map[string]bool)
	endedKey     = make(map[string]bool)
)

func main() {
	// Config + storage
	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s <config.json>", os.Args[0])
	}
	if err := loadConfig(os.Args[1]); err != nil {
		log.Fatalf("config: %v", err)
	}
	var err error
	loc, err = time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Fatalf("timezone: %v", err)
	}
	if err := loadStore(); err != nil {
		log.Fatalf("store: %v", err)
	}

	// Discord
	dg, err = discordgo.New("Bot " + cfg.Token)
	if err != nil {
		log.Fatalf("discord: %v", err)
	}
	dg.Identify.Intents = discordgo.IntentsGuilds
	dg.AddHandler(onInteractionCreate)

	if err := dg.Open(); err != nil {
		log.Fatalf("open ws: %v", err)
	}
	defer dg.Close()

	// Register slash command
	cmd := &discordgo.ApplicationCommand{
		Name:        "helpdesk",
		Description: "Manage the helpdesk role",
		Options: []*discordgo.ApplicationCommandOption{
			{Name: "join", Description: "Join helpdesk", Type: discordgo.ApplicationCommandOptionSubCommand},
			{Name: "leave", Description: "Leave helpdesk", Type: discordgo.ApplicationCommandOptionSubCommand},
			{Name: "list", Description: "Show current and next shifts", Type: discordgo.ApplicationCommandOptionSubCommand},
			{
				Name:        "override",
				Description: "Admin: force add/remove a user",
				Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
				Options: []*discordgo.ApplicationCommandOption{
					{
						Name:        "add",
						Description: "Force add user to role now",
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Options: []*discordgo.ApplicationCommandOption{
							{Name: "user", Description: "User", Type: discordgo.ApplicationCommandOptionUser, Required: true},
						},
					},
					{
						Name:        "remove",
						Description: "Force remove user from role now",
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Options: []*discordgo.ApplicationCommandOption{
							{Name: "user", Description: "User", Type: discordgo.ApplicationCommandOptionUser, Required: true},
						},
					},
				},
			},
			{Name: "sync", Description: "Reconcile role with schedule (manual)", Type: discordgo.ApplicationCommandOptionSubCommand},
		},
	}
	created, err := dg.ApplicationCommandCreate(dg.State.User.ID, cfg.GuildID, cmd)
	if err != nil {
		log.Printf("register command: %v", err)
	} else {
		log.Printf("command /%s registered (id=%s)", created.Name, created.ID)
	}

	go schedulerLoop()

	// Wait
	log.Printf("running. guild=%s role=%s tz=%s", cfg.GuildID, cfg.RoleID, cfg.Timezone)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down")
}

func loadConfig(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return err
	}
	if cfg.Token == "" || cfg.GuildID == "" || cfg.RoleID == "" || cfg.Timezone == "" || cfg.DataFile == "" {
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
			store = Store{Volunteers: []string{}, Schedule: []Shift{}}
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
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, cfg.DataFile)
}

func onInteractionCreate(s *discordgo.Session, ic *discordgo.InteractionCreate) {
	if ic.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if ic.ApplicationCommandData().Name != "helpdesk" {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic in handler: %v", r)
		}
	}()

	data := ic.ApplicationCommandData()
	if len(data.Options) == 0 {
		reply(s, ic, "No subcommand.")
		return
	}
	opt := data.Options[0]

	switch opt.Name {
	case "join":
		userID := ic.Member.User.ID
		if hasRoleNow(userID) {
			reply(s, ic, "You already have the helpdesk role.")
			return
		}
		if err := addRole(userID); err != nil {
			reply(s, ic, "Failed to add role (permissions?): "+err.Error())
			return
		}
		if !contains(store.Volunteers, userID) {
			store.Volunteers = append(store.Volunteers, userID)
			_ = saveStore()
		}
		reply(s, ic, "You are on helpdesk.")

	case "leave":
		userID := ic.Member.User.ID
		if contains(store.Volunteers, userID) {
			store.Volunteers = remove(store.Volunteers, userID)
			_ = saveStore()
		}
		if !isUserScheduledNow(userID) {
			if err := removeRole(userID); err != nil {
				reply(s, ic, "Failed to remove role (permissions?): "+err.Error())
				return
			}
		}
		reply(s, ic, "You have left helpdesk.")

	case "list":
		now := time.Now().In(loc)
		cur, next := currentAndNextShift(now)
		var b strings.Builder
		fmt.Fprintf(&b, "**Now:** %s\n", mentionUsers(cur.UserIDs))
		fmt.Fprintf(&b, "Time: %s -> %s\n", cur.Start, cur.End)
		if len(cur.UserIDs) == 0 {
			fmt.Fprintf(&b, "_No active shifts._\n")
		}
		fmt.Fprintf(&b, "\n**Next:** %s\n", mentionUsers(next.UserIDs))
		fmt.Fprintf(&b, "Time: %s -> %s\n", next.Start, next.End)
		if len(store.Volunteers) > 0 {
			fmt.Fprintf(&b, "\n**Volunteers:** %s\n", mentionUsers(store.Volunteers))
		}
		reply(s, ic, b.String())

	case "override":
		if !isAdmin(ic) {
			reply(s, ic, "Admin only.")
			return
		}
		if len(opt.Options) == 0 {
			reply(s, ic, "Missing subcommand.")
			return
		}
		sub := opt.Options[0]
		user := sub.Options[0].UserValue(s)
		if user == nil {
			reply(s, ic, "User not found.")
			return
		}
		switch sub.Name {
		case "add":
			if err := addRole(user.ID); err != nil {
				reply(s, ic, "Failed to add role: "+err.Error())
				return
			}
			reply(s, ic, fmt.Sprintf("Added %s to helpdesk role.", mentionUsers([]string{user.ID})))
		case "remove":
			if err := removeRole(user.ID); err != nil {
				reply(s, ic, "Failed to remove role: "+err.Error())
				return
			}
			if contains(store.Volunteers, user.ID) {
				store.Volunteers = remove(store.Volunteers, user.ID)
				_ = saveStore()
			}
			reply(s, ic, fmt.Sprintf("Removed %s from helpdesk role.", mentionUsers([]string{user.ID})))
		default:
			reply(s, ic, "Unknown override action.")
		}

	case "sync":
		now := time.Now().In(loc)
		applied, errs := applyShiftState(now)
		msg := "Sync done." + applied
		if len(errs) > 0 {
			msg += "\nError:\n- " + strings.Join(errs, "\n-")
		}
		reply(s, ic, msg)

	default:
		reply(s, ic, "Unknown subcommand.")
	}
}

func reply(s *discordgo.Session, ic *discordgo.InteractionCreate, content string) {
	_ = s.InteractionRespond(ic.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Flags: 1 << 6},
	})
}

func isAdmin(ic *discordgo.InteractionCreate) bool {
	// Check ADMINISTRATOR bit (0x00000008)
	// If unknown, deny by default.
	const PermAdmin = 0x00000008
	return ic.Member != nil && (ic.Member.Permissions&PermAdmin) == PermAdmin
}

// Role ops

func addRole(userID string) error {
	return dg.GuildMemberRoleAdd(cfg.GuildID, userID, cfg.RoleID)
}

func removeRole(userID string) error {
	return dg.GuildMemberRoleRemove(cfg.GuildID, userID, cfg.RoleID)
}

func hasRoleNow(userID string) bool {
	m, err := dg.GuildMember(cfg.GuildID, userID)
	if err != nil {
		return false
	}
	return slices.Contains(m.Roles, cfg.RoleID)
}

// Schedule helpers

func parse(t string) (time.Time, error) {
	tt, err := time.ParseInLocation(time.RFC3339, t, loc)
	if err != nil {
		tt, err2 := time.ParseInLocation("2006-01-02 15:04", t, loc)
		if err2 != nil {
			return time.Time{}, err
		}
		return tt, nil
	}
	return tt, nil
}

func schedulerLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		now := time.Now().In(loc)
		_, _ = applyShiftState(now)
		<-ticker.C
	}
}

func applyShiftState(now time.Time) (string, []string) {
	var errs []string
	type event struct{ action, userID, when string }
	var applied []event

	for _, sh := range store.Schedule {
		start, err1 := parse(sh.Start)
		end, err2 := parse(sh.End)
		if err1 != nil || err2 != nil {
			if err1 != nil {
				errs = append(errs, "parse start: "+err1.Error())
			}
			if err2 != nil {
				errs = append(errs, "parse end: "+err2.Error())
			}
			continue
		}

		for _, uid := range sh.UserIDs {
			rKey := sh.Start + "|" + uid
			aKey := sh.Start + "|" + uid
			eKey := sh.End + "|" + uid

			// 1) Remind T-20m (once)
			if start.After(now) && start.Sub(now) <= 20*time.Minute && !remindedKey[rKey] {
				if err := remindUser(uid, start); err != nil {
					errs = append(errs, "remind "+uid+": "+err.Error())
				} else {
					remindedKey[rKey] = true
					applied = append(applied, event{"reminded", uid, sh.Start})
				}
			}
			// 2) Activate at start (once)
			if (now.Equal(start) || (now.After(start) && now.Before(end))) && !activatedKey[aKey] {
				if !hasRoleNow(uid) {
					if err := addRole(uid); err != nil {
						errs = append(errs, "add role "+uid+": "+err.Error())
					} else {
						applied = append(applied, event{"activated", uid, sh.Start})
					}
				}
				activatedKey[aKey] = true
			}
			// 3) Deactivate at end (once) unless user is a volunteer
			if now.After(end) && !endedKey[eKey] {
				if !contains(store.Volunteers, uid) && hasRoleNow(uid) {
					if err := removeRole(uid); err != nil {
						errs = append(errs, "remove role "+uid+": "+err.Error())
					} else {
						applied = append(applied, event{"ended", uid, sh.End})
					}
				}
				endedKey[eKey] = true
			}
		}
	}
	// brief summary
	if len(applied) == 0 {
		return "no changes", errs
	}
	var parts []string
	for _, a := range applied {
		parts = append(parts, fmt.Sprintf("%s %s (%s)", a.action, a.userID, a.when))
	}
	return strings.Join(parts, ", "), errs
}

func remindUser(userID string, start time.Time) error {
	ch, err := dg.UserChannelCreate(userID)
	if err != nil {
		return err
	}
	_, err = dg.ChannelMessageSend(ch.ID,
		fmt.Sprintf("Reminder: your helpdesk shift starts at **%s** (%s). You'll be added to the role automatically.",
			start.Format("15:04"), cfg.Timezone))
	return err
}

func isUserScheduledNow(userID string) bool {
	now := time.Now().In(loc)
	for _, sh := range store.Schedule {
		start, err1 := parse(sh.Start)
		end, err2 := parse(sh.End)
		if err1 != nil || err2 != nil {
			continue
		}
		if (now.Equal(start) || (now.After(start) && now.Before(end))) && contains(sh.UserIDs, userID) {
			return true
		}
	}
	return false
}

func currentAndNextShift(now time.Time) (Shift, Shift) {
	type parsed struct {
		Shift
		pStart time.Time
	}
	var parsedSh []parsed
	for _, sh := range store.Schedule {
		ps, err1 := parse(sh.Start)
		pe, err2 := parse(sh.End)
		if err1 != nil || err2 != nil {
			continue
		}
		if pe.Before(ps) {
			continue
		}
		parsedSh = append(parsedSh, parsed{sh, ps})
	}
	sort.Slice(parsedSh, func(i, j int) bool { return parsedSh[i].pStart.Before(parsedSh[j].pStart) })

	var cur Shift
	var next Shift
	foundCur := false
	for _, p := range parsedSh {
		ps, _ := parse(p.Start)
		pe, _ := parse(p.End)
		if (now.Equal(ps) || (now.After(ps) && now.Before(pe))) && !foundCur {
			cur = p.Shift
			foundCur = true
		}
		if ps.After(now) {
			next = p.Shift
			break
		}
	}
	return cur, next
}

// Utils

func mentionUsers(ids []string) string {
	if len(ids) == 0 {
		return "(none)"
	}
	var parts []string
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			parts = append(parts, "<@"+id+">")
		}
	}
	return strings.Join(parts, "")
}

func contains(arr []string, v string) bool {
	return slices.Contains(arr, v)
}

func remove(arr []string, v string) []string {
	out := arr[:0]
	for _, x := range arr {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

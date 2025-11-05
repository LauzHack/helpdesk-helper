// Package scheduler/schedule.go
package scheduler

import (
	"fmt"
	"lauzhack-bot/botapi"
	"lauzhack-bot/config"
	"lauzhack-bot/data"
	"lauzhack-bot/utils"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	bot          botapi.BotAPI
	remindedKey  = make(map[string]bool)
	activatedKey = make(map[string]bool)
	endedKey     = make(map[string]bool)
)

func Init(b botapi.BotAPI) {
	bot = b
}

func SchedulerLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		now := time.Now()
		cleanupState(now)
		_, _ = ApplyShiftState(now)
		<-ticker.C
	}
}

func cleanupState(now time.Time) {
	cutoff := now.Add(-2 * time.Hour).Unix() // anything ending >2h ago
	for k := range remindedKey {
		parts := strings.Split(k, "|")
		if len(parts) < 1 {
			continue
		}
		if ts, err := strconv.ParseInt(parts[0], 10, 64); err == nil && ts < cutoff {
			delete(remindedKey, k)
		}
	}
	for k := range activatedKey {
		parts := strings.Split(k, "|")
		if len(parts) < 1 {
			continue
		}
		if ts, err := strconv.ParseInt(parts[0], 10, 64); err == nil && ts < cutoff {
			delete(activatedKey, k)
		}
	}
	for k := range endedKey {
		parts := strings.Split(k, "|")
		if len(parts) < 1 {
			continue
		}
		if ts, err := strconv.ParseInt(parts[0], 10, 64); err == nil && ts < cutoff {
			delete(endedKey, k)
		}
	}
}

func activeShiftsForUser(userID string, now time.Time) []data.Shift {
	var res []data.Shift
	store := config.GetStore()
	for _, sh := range store.Schedule {
		start := time.Unix(sh.Start, 0)
		end := time.Unix(sh.End, 0)
		if (now.Equal(start) || (now.After(start) && now.Before(end))) && utils.Contains(sh.UserIDs, userID) {
			res = append(res, sh)
		}
	}
	return res
}

func IsUserScheduledNow(userID string) bool {
	return len(activeShiftsForUser(userID, time.Now())) > 0
}

func ApplyShiftState(now time.Time) (string, []string) {
	var errs []string
	type event struct {
		action string
		userID string
		when   int64
	}
	var applied []event

	store := config.GetStore()

	// Main schedule loop
	for _, sh := range store.Schedule {
		start := time.Unix(sh.Start, 0)
		end := time.Unix(sh.End, 0)

		for _, uid := range sh.UserIDs {
			rkey := fmt.Sprintf("%d|%s|remind", sh.Start, uid)
			ekey := fmt.Sprintf("%d|%s|end", sh.End, uid)

			// Reminder (T-20m)
			if start.After(now) && start.Sub(now) <= 20*time.Minute && !remindedKey[rkey] {
				if err := bot.SendReminder(uid, start); err != nil {
					errs = append(errs, "remind "+uid+": "+err.Error())
				} else {
					remindedKey[rkey] = true
					applied = append(applied, event{"reminded", uid, sh.Start})
				}
			}

			// Activation (active window)
			if now.After(start.Add(-10*time.Second)) && now.Before(end) {
				if !bot.HasRoleNow(uid) {
					if err := bot.AddRole(uid); err != nil {
						errs = append(errs, "add role "+uid+": "+err.Error())
					} else {
						applied = append(applied, event{"activated", uid, sh.Start})
					}
				}
			}

			// Deactivation (after end)
			if now.After(end) && !endedKey[ekey] {
				if !utils.Contains(store.Volunteers, uid) && bot.HasRoleNow(uid) {
					if err := bot.RemoveRole(uid); err != nil {
						errs = append(errs, "remove role "+uid+": "+err.Error())
					} else {
						applied = append(applied, event{"ended", uid, sh.End})
					}
				}
				endedKey[ekey] = true
			}
		}
	}

	// Reconciliation pass
	guildMembers, err := bot.GetAllMembers(config.Cfg.GuildID)
	if err == nil {
		nowUnix := now.Unix()
		var shouldHave []string

		// Gather all active + volunteer IDs
		for _, sh := range store.Schedule {
			if nowUnix >= sh.Start && nowUnix <= sh.End {
				shouldHave = append(shouldHave, sh.UserIDs...)
			}
		}
		shouldHave = append(shouldHave, store.Volunteers...)

		for _, m := range guildMembers {
			if slices.Contains(m.Roles, config.Cfg.RoleID) && !slices.Contains(shouldHave, m.User.ID) {
				if err := bot.RemoveRole(m.User.ID); err != nil {
					errs = append(errs, fmt.Sprintf("reconcile remove %s: %v", m.User.ID, err))
				} else {
					applied = append(applied, event{"reconciled-remove", m.User.ID, nowUnix})
				}
			}
		}
	} else {
		errs = append(errs, "failed to fetch guild members for reconciliation: "+err.Error())
	}

	// Generate summary
	if len(applied) == 0 {
		return "✅ No changes needed — everything is up to date.", errs
	}

	var (
		added    []string
		removed  []string
		reminded []string
	)

	for _, a := range applied {
		label := map[string]string{
			"activated":         "🟢 Activated",
			"ended":             "🔴 Deactivated",
			"reminded":          "🕐 Reminder sent",
			"reconciled-remove": "⚪ Removed (not scheduled)",
		}[a.action]
		entry := fmt.Sprintf("%s <@%s> (<t:%d:R>)", label, a.userID, a.when)
		switch a.action {
		case "activated":
			added = append(added, entry)
		case "ended", "reconciled-remove":
			removed = append(removed, entry)
		case "reminded":
			reminded = append(reminded, entry)
		}
	}

	var b strings.Builder
	b.WriteString("**Helpdesk sync summary**\n\n")
	if len(reminded) > 0 {
		b.WriteString(strings.Join(reminded, "\n") + "\n\n")
	}
	if len(added) > 0 {
		b.WriteString(strings.Join(added, "\n") + "\n\n")
	}
	if len(removed) > 0 {
		b.WriteString(strings.Join(removed, "\n") + "\n\n")
	}

	return b.String(), errs
}

func CurrentAndNextShift(now time.Time) (data.Shift, data.Shift) {
	type parsed struct {
		data.Shift
		pStart time.Time
	}
	store := config.GetStore()
	var parsedSh []parsed
	for _, sh := range store.Schedule {
		ps := time.Unix(sh.Start, 0)
		pe := time.Unix(sh.End, 0)
		if pe.Before(ps) {
			continue
		}
		parsedSh = append(parsedSh, parsed{sh, ps})
	}
	sort.Slice(parsedSh, func(i, j int) bool { return parsedSh[i].pStart.Before(parsedSh[j].pStart) })

	var cur data.Shift
	var next data.Shift
	foundCur := false
	for _, p := range parsedSh {
		ps := time.Unix(p.Start, 0)
		pe := time.Unix(p.End, 0)
		if (now.Equal(ps) || (now.After(ps) && now.Before(pe))) && !foundCur {
			cur = p.Shift
			foundCur = true
		}
		if ps.After(now) {
			next = p.Shift
			break
		}
	}

	if !foundCur {
		cur = data.Shift{}
	}

	if next.Start == 0 && next.End == 0 {
		next = data.Shift{Start: 0, End: 0, UserIDs: nil}
	}

	return cur, next
}

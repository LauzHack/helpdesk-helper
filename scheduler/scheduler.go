// Package scheduler
package scheduler

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"lauzhack-bot/botapi"
	"lauzhack-bot/config"
	"lauzhack-bot/store"
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

	cleanup := func(m map[string]bool) {
		for k := range m {
			parts := strings.Split(k, "|")
			if len(parts) < 1 {
				continue
			}
			if ts, err := strconv.ParseInt(parts[0], 10, 64); err == nil && ts < cutoff {
				delete(m, k)
			}
		}
	}

	cleanup(remindedKey)
	cleanup(activatedKey)
	cleanup(endedKey)
}

func activeShiftsForUser(userID string, now time.Time) []store.Shift {
	var res []store.Shift
	shifts := store.ListShifts()
	for _, sh := range shifts {
		start := time.Unix(sh.Start, 0)
		end := time.Unix(sh.End, 0)
		if now.After(start) && now.Before(end) && slices.Contains(sh.UserIDs, userID) {
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

	volunteers := store.ListVolunteers()
	shifts := store.ListShifts()

	for _, sh := range shifts {
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

			// Activation (during shift)
			if now.After(start.Add(-10*time.Second)) && now.Before(end) {
				if !bot.HasRoleNow(uid) {
					if err := bot.AddRole(uid); err != nil {
						errs = append(errs, "add role "+uid+": "+err.Error())
					} else {
						applied = append(applied, event{"activated", uid, sh.Start})
					}
				}
			}

			// Deactivation (after shift)
			if now.After(end) && !endedKey[ekey] {
				if bot.HasRoleNow(uid) {
					if err := bot.RemoveRole(uid); err != nil {
						errs = append(errs, "remove role "+uid+": "+err.Error())
					} else {
						applied = append(applied, event{"ended", uid, sh.End})
					}
				}
				endedKey[ekey] = true
				_ = store.RemoveVolunteer(uid)  // remove served volunteer
				_ = store.RemoveShift(sh.Start) // remove completed shift
			}
		}
	}

	guildMembers, err := bot.GetAllMembers(config.Cfg.GuildID)
	if err == nil {
		nowUnix := now.Unix()
		var shouldHave []string

		for _, sh := range shifts {
			if nowUnix >= sh.Start && nowUnix <= sh.End {
				shouldHave = append(shouldHave, sh.UserIDs...)
			}
		}
		shouldHave = append(shouldHave, volunteers...)

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

	if len(applied) == 0 {
		return "✅ No changes needed — everything is up to date.", errs
	}

	var reminded, added, removed []string
	for _, a := range applied {
		label := map[string]string{
			"activated":         "🟢 Activated",
			"ended":             "🔴 Deactivated",
			"reminded":          "🕐 Reminder sent",
			"reconciled-remove": "⚪ Removed (not scheduled)",
		}[a.action]
		entry := fmt.Sprintf("%s <@%s> (<t:%d:R>)", label, a.userID, a.when)
		switch a.action {
		case "reminded":
			reminded = append(reminded, entry)
		case "activated":
			added = append(added, entry)
		case "ended", "reconciled-remove":
			removed = append(removed, entry)
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

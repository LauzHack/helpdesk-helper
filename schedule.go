package main

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"lauzhack-bot/data"
)

func schedulerLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		now := time.Now().In(loc)
		cleanupState(now)
		_, _ = applyShiftState(now)
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
	for _, sh := range store.Schedule {
		start := time.Unix(sh.Start, 0).In(loc)
		end := time.Unix(sh.End, 0).In(loc)
		if (now.Equal(start) || (now.After(start) && now.Before(end))) && contains(sh.UserIDs, userID) {
			res = append(res, sh)
		}
	}
	return res
}

func isUserScheduledNow(userID string) bool {
	return len(activeShiftsForUser(userID, time.Now().In(loc))) > 0
}

func applyShiftState(now time.Time) (string, []string) {
	var errs []string
	type event struct {
		action string
		userID string
		when   int64
	}
	var applied []event

	// Main schedule loop
	for _, sh := range store.Schedule {
		start := time.Unix(sh.Start, 0).In(loc)
		end := time.Unix(sh.End, 0).In(loc)

		for _, uid := range sh.UserIDs {
			rkey := fmt.Sprintf("%d|%s|remind", sh.Start, uid)
			ekey := fmt.Sprintf("%d|%s|end", sh.End, uid)

			// Reminder (T-20m)
			if start.After(now) && start.Sub(now) <= 20*time.Minute && !remindedKey[rkey] {
				if err := remindUser(uid, start); err != nil {
					errs = append(errs, "remind "+uid+": "+err.Error())
				} else {
					remindedKey[rkey] = true
					applied = append(applied, event{"reminded", uid, sh.Start})
				}
			}

			// Activation (active window)
			if now.After(start.Add(-10*time.Second)) && now.Before(end) {
				if !hasRoleNow(uid) {
					if err := addRole(uid); err != nil {
						errs = append(errs, "add role "+uid+": "+err.Error())
					} else {
						applied = append(applied, event{"activated", uid, sh.Start})
					}
				}
			}

			// Deactivation (after end)
			if now.After(end) && !endedKey[ekey] {
				if !contains(store.Volunteers, uid) && hasRoleNow(uid) {
					if err := removeRole(uid); err != nil {
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
	guildMembers, err := getAllMembers(cfg.GuildID)
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
			if slices.Contains(m.Roles, cfg.RoleID) && !slices.Contains(shouldHave, m.User.ID) {
				if err := removeRole(m.User.ID); err != nil {
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

func remindUser(userID string, start time.Time) error {
	const reminderChannelID = "888853355747770429"
	msg := fmt.Sprintf(
		"<@%s> Reminder: your helpdesk shift starts <t:%d:R> (%s). Please head to the helpdesk desk during that timeframe.",
		userID, start.Unix(), cfg.Timezone,
	)
	_, err := dg.ChannelMessageSend(reminderChannelID, msg)
	return err
}

func currentAndNextShift(now time.Time) (data.Shift, data.Shift) {
	type parsed struct {
		data.Shift
		pStart time.Time
	}
	var parsedSh []parsed
	for _, sh := range store.Schedule {
		ps := time.Unix(sh.Start, 0).In(loc)
		pe := time.Unix(sh.End, 0).In(loc)
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
		ps := time.Unix(p.Start, 0).In(loc)
		pe := time.Unix(p.End, 0).In(loc)
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

package handlers

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"socialmedia/internal/models"
)

// defaultAgentTimezone is the zone an agent schedule is read in when it names
// none; the same default as get_open_slots.
const defaultAgentTimezone = "Europe/Berlin"

// minAgentInterval keeps an interval schedule from burning through the
// user's OpenRouter credit.
const minAgentInterval = 15

// normalizeAgentSchedule validates a schedule and returns it cleaned up:
// times parsed and sorted, weekdays deduplicated, the timezone defaulted.
func normalizeAgentSchedule(s models.AgentSchedule) (models.AgentSchedule, error) {
	out := models.AgentSchedule{Kind: s.Kind, Timezone: strings.TrimSpace(s.Timezone)}
	if out.Timezone == "" {
		out.Timezone = defaultAgentTimezone
	}
	if _, err := time.LoadLocation(out.Timezone); err != nil {
		return out, fmt.Errorf("unknown timezone %q", out.Timezone)
	}
	switch s.Kind {
	case models.AgentScheduleManual:
	case models.AgentScheduleInterval:
		if s.IntervalMinutes < minAgentInterval {
			return out, fmt.Errorf("the interval must be at least %d minutes", minAgentInterval)
		}
		if s.IntervalMinutes > 31*24*60 {
			return out, fmt.Errorf("the interval must be at most 31 days")
		}
		out.IntervalMinutes = s.IntervalMinutes
	case models.AgentScheduleHourly:
		if s.Minute < 0 || s.Minute > 59 {
			return out, fmt.Errorf("the minute must be between 0 and 59")
		}
		out.Minute = s.Minute
	case models.AgentScheduleDaily:
		seen := map[string]bool{}
		for _, t := range s.Times {
			t = strings.TrimSpace(t)
			parsed, err := time.Parse("15:04", t)
			if err != nil {
				return out, fmt.Errorf("invalid time %q, use HH:MM", t)
			}
			t = parsed.Format("15:04")
			if !seen[t] {
				seen[t] = true
				out.Times = append(out.Times, t)
			}
		}
		if len(out.Times) == 0 {
			return out, fmt.Errorf("add at least one time of day")
		}
		sort.Strings(out.Times)
		days := map[int]bool{}
		for _, d := range s.Weekdays {
			if d < 0 || d > 6 {
				return out, fmt.Errorf("weekdays run from 0 (Monday) to 6 (Sunday)")
			}
			if !days[d] {
				days[d] = true
				out.Weekdays = append(out.Weekdays, d)
			}
		}
		sort.Ints(out.Weekdays)
		// All seven days is the same as none: every day.
		if len(out.Weekdays) == 7 {
			out.Weekdays = nil
		}
	default:
		return out, fmt.Errorf("unknown schedule kind %q", s.Kind)
	}
	return out, nil
}

// nextAgentRun returns the first run time of a (normalized) schedule strictly
// after `after`, or nil for a manual schedule.
func nextAgentRun(s models.AgentSchedule, after time.Time) *time.Time {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil || s.Timezone == "" {
		loc, _ = time.LoadLocation(defaultAgentTimezone)
		if loc == nil {
			loc = time.UTC
		}
	}
	var next time.Time
	switch s.Kind {
	case models.AgentScheduleInterval:
		if s.IntervalMinutes <= 0 {
			return nil
		}
		next = after.Add(time.Duration(s.IntervalMinutes) * time.Minute)
	case models.AgentScheduleHourly:
		local := after.In(loc)
		next = time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), s.Minute, 0, 0, loc)
		for !next.After(after) {
			next = next.Add(time.Hour)
		}
	case models.AgentScheduleDaily:
		days := map[int]bool{}
		for _, d := range s.Weekdays {
			days[d] = true
		}
		local := after.In(loc)
		for i := 0; i <= 8; i++ {
			day := time.Date(local.Year(), local.Month(), local.Day()+i, 0, 0, 0, 0, loc)
			weekday := (int(day.Weekday()) + 6) % 7 // 0 = Monday
			if len(days) > 0 && !days[weekday] {
				continue
			}
			for _, t := range s.Times {
				hm, err := time.Parse("15:04", t)
				if err != nil {
					continue
				}
				cand := time.Date(day.Year(), day.Month(), day.Day(), hm.Hour(), hm.Minute(), 0, 0, loc)
				if cand.After(after) {
					next = cand
					break
				}
			}
			if !next.IsZero() {
				break
			}
		}
		if next.IsZero() {
			return nil
		}
	default:
		return nil
	}
	next = next.UTC()
	return &next
}

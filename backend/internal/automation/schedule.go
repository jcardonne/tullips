// Package automation contains deterministic, side-effect-free workflow decisions.
package automation

import (
	"errors"
	"time"
)

type Window struct {
	WorkingDays []int  `json:"working_days,omitempty"`
	Timezone    string `json:"timezone"`
	StartHour   int    `json:"startHour"`
	EndHour     int    `json:"endHour"`
}

func DefaultWindow() Window { return Window{Timezone: "Europe/Paris", StartHour: 9, EndHour: 18} }
func (w Window) location() (*time.Location, error) {
	if w.StartHour < 0 || w.EndHour > 24 || w.StartHour >= w.EndHour {
		return nil, errors.New("invalid activity hours")
	}
	if w.WorkingDays != nil {
		if len(w.WorkingDays) == 0 {
			return nil, errors.New("at least one activity day required")
		}
		for _, day := range w.WorkingDays {
			if day < 0 || day > 6 {
				return nil, errors.New("invalid activity day")
			}
		}
	}
	return time.LoadLocation(w.Timezone)
}
func (w Window) activeDay(t time.Time) bool {
	if w.WorkingDays == nil {
		return t.Weekday() != time.Saturday && t.Weekday() != time.Sunday
	}
	for _, d := range w.WorkingDays {
		if int(t.Weekday()) == d {
			return true
		}
	}
	return false
}
func NextActive(t time.Time, w Window) (time.Time, error) {
	loc, err := w.location()
	if err != nil {
		return time.Time{}, err
	}
	t = t.In(loc)
	for {
		if w.activeDay(t) && t.Hour() >= w.StartHour && t.Hour() < w.EndHour {
			return t, nil
		}
		if w.activeDay(t) && t.Hour() < w.StartHour {
			return time.Date(t.Year(), t.Month(), t.Day(), w.StartHour, 0, 0, 0, loc), nil
		}
		t = time.Date(t.Year(), t.Month(), t.Day()+1, w.StartHour, 0, 0, 0, loc)
	}
}

// BusinessDays preserves local clock time across daylight-saving changes.
func BusinessDays(t time.Time, days int, w Window) (time.Time, error) {
	if days < 0 {
		return time.Time{}, errors.New("negative delay")
	}
	loc, err := w.location()
	if err != nil {
		return time.Time{}, err
	}
	t = t.In(loc)
	for days > 0 {
		t = t.AddDate(0, 0, 1)
		if w.activeDay(t) {
			days--
		}
	}
	return NextActive(t, w)
}
func DailyLimit(activated, now time.Time) int {
	days := now.Sub(activated).Hours() / 24
	if days >= 60 {
		return 100
	}
	if days >= 14 {
		return 60
	}
	return 40
}
func FirstMessage(invited, accepted time.Time, w Window) (time.Time, error) {
	if accepted.IsZero() {
		return time.Time{}, errors.New("connection not accepted")
	}
	due, err := BusinessDays(invited, 2, w)
	if err != nil {
		return due, err
	}
	if accepted.After(due) {
		due = accepted
	}
	return NextActive(due, w)
}
func InvitationExpired(invited, now time.Time) bool { return !now.Before(invited.AddDate(0, 0, 30)) }

type Facts struct {
	Accepted, Replied, HasEmail, EmailOpened, LinkClicked, MessageSeen bool
	Tags                                                               []string
	Status                                                             string
	AI                                                                 *bool
}
type Condition struct {
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
}

// Missing AI decisions and unsupported events fail closed, never imply permission to send.
func Evaluate(c Condition, f Facts) (bool, error) {
	switch c.Kind {
	case "accepted":
		return f.Accepted, nil
	case "not_accepted":
		return !f.Accepted, nil
	case "has_email":
		return f.HasEmail, nil
	case "email_opened":
		return f.EmailOpened, nil
	case "link_clicked":
		return f.LinkClicked, nil
	case "message_seen":
		return f.MessageSeen, nil
	case "status":
		return f.Status == c.Value, nil
	case "tag":
		for _, t := range f.Tags {
			if t == c.Value {
				return true, nil
			}
		}
		return false, nil
	case "ai":
		if f.AI == nil {
			return false, errors.New("AI decision pending")
		}
		return *f.AI, nil
	case "no_reply":
		return !f.Replied, nil
	default:
		return false, errors.New("unsupported condition")
	}
}
func MaySend(f Facts) bool { return !f.Replied }

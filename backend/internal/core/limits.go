package core

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

func validateRamp(settings map[string]any) error {
	days, hasDays := settings["ramp_days"]
	limits, hasLimits := settings["ramp_limits"]
	if !hasDays && !hasLimits {
		return nil
	}
	d, okD := days.([]any)
	l, okL := limits.([]any)
	if !okD || !okL || len(d) != len(l) || len(d) == 0 || len(d) > 10 {
		return errors.New("ramp_days and ramp_limits must have matching 1 to 10 stages")
	}
	previous := -1
	for i, v := range d {
		day, ok := v.(float64)
		limit, okLimit := l[i].(float64)
		if !ok || !okLimit || day != float64(int(day)) || limit != float64(int(limit)) || int(day) <= previous || limit < 1 || day > 3650 {
			return errors.New("ramp days must increase and limits must be positive integers")
		}
		if i == 0 && day != 0 {
			return errors.New("first ramp stage must start on day 0")
		}
		previous = int(day)
	}
	return nil
}
func rampLimit(account, settings map[string]any, activated, now time.Time) int {
	if n := number(account, "daily_limit", 0); n > 0 {
		return n
	}
	days := []int{0, 14, 60}
	limits := []int{40, 60, 100}
	if validateRamp(settings) == nil {
		if configured, ok := settings["ramp_days"].([]any); ok {
			days = []int{}
			limits = []int{}
			for i, d := range configured {
				days = append(days, int(d.(float64)))
				limits = append(limits, int(settings["ramp_limits"].([]any)[i].(float64)))
			}
		}
	}
	age := int(now.Sub(activated).Hours() / 24)
	limit := limits[0]
	for i, day := range days {
		if age >= day {
			limit = limits[i]
		}
	}
	return limit
}
func (s *Server) accountLimit(ctx context.Context, ws string, account map[string]any, activated time.Time) (int, error) {
	var raw []byte
	if e := s.DB.QueryRow(ctx, `SELECT settings FROM workspaces WHERE id=$1`, ws).Scan(&raw); e != nil {
		return 0, e
	}
	var settings map[string]any
	json.Unmarshal(raw, &settings)
	return rampLimit(account, settings, activated, time.Now()), nil
}

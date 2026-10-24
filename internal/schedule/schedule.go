/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package schedule

import (
	"fmt"
	"time"
	_ "time/tzdata"

	"github.com/robfig/cron/v3"
)

var cronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// Parse parses a standard 5-field cron expression evaluated in the given IANA time zone.
// If timeZone is empty, "UTC" is used.
func Parse(scheduleStr, timeZoneStr string) (cron.Schedule, *time.Location, error) {
	if timeZoneStr == "" {
		timeZoneStr = "UTC"
	}
	loc, err := time.LoadLocation(timeZoneStr)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid time zone %q: %w", timeZoneStr, err)
	}

	expr := fmt.Sprintf("CRON_TZ=%s %s", timeZoneStr, scheduleStr)
	sched, err := cronParser.Parse(expr)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid cron expression %q: %w", scheduleStr, err)
	}
	return sched, loc, nil
}

// NextRun computes the next scheduled run strictly after `from`.
func NextRun(scheduleStr, timeZoneStr string, from time.Time) (time.Time, error) {
	sched, loc, err := Parse(scheduleStr, timeZoneStr)
	if err != nil {
		return time.Time{}, err
	}
	next := sched.Next(from.In(loc))
	return next, nil
}

// LastScheduledTime computes the most recent scheduled run time that occurred
// at or before `now`. If no scheduled time occurred in the last 5 years, it returns
// a zero time.
func LastScheduledTime(scheduleStr, timeZoneStr string, now time.Time) (time.Time, error) {
	sched, loc, err := Parse(scheduleStr, timeZoneStr)
	if err != nil {
		return time.Time{}, err
	}

	nowInLoc := now.In(loc)

	// Step backwards using increasing lookback windows to find a starting point T0
	// where sched.Next(T0) <= nowInLoc.
	lookbacks := []time.Duration{
		2 * time.Minute,
		1 * time.Hour,
		24 * time.Hour,
		7 * 24 * time.Hour,
		31 * 24 * time.Hour,
		366 * 24 * time.Hour,
		5 * 366 * 24 * time.Hour,
	}

	var start time.Time
	found := false
	for _, lb := range lookbacks {
		t0 := nowInLoc.Add(-lb)
		first := sched.Next(t0)
		if !first.IsZero() && !first.After(nowInLoc) {
			start = t0
			found = true
			break
		}
	}

	if !found {
		return time.Time{}, nil
	}

	// Iterate forward from start until the next run would be after nowInLoc.
	// The last run at or before nowInLoc is our target.
	var last time.Time
	curr := start
	for {
		next := sched.Next(curr)
		if next.IsZero() || next.After(nowInLoc) {
			break
		}
		last = next
		curr = next
	}

	return last, nil
}

// IsDue checks if a run is due based on the schedule, time zone, lastRunID, and current time.
// According to docs/ARCHITECTURE.md:
// "A run is due when the most recent scheduled time (from spec.schedule and spec.timeZone)
// is later than status.lastRunID. If several schedules were missed (controller down),
// only the most recent one runs."
//
// Returns:
// - due: true if a run should be started
// - scheduledTime: the most recent scheduled time (whose Unix timestamp is the runID)
// - err: any error parsing the schedule or timezone
func IsDue(scheduleStr, timeZoneStr string, lastRunID int64, now time.Time) (bool, time.Time, error) {
	lastSched, err := LastScheduledTime(scheduleStr, timeZoneStr, now)
	if err != nil {
		return false, time.Time{}, err
	}
	if lastSched.IsZero() {
		return false, time.Time{}, nil
	}

	if lastRunID >= lastSched.Unix() {
		return false, lastSched, nil
	}

	return true, lastSched, nil
}

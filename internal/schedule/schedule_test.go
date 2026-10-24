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
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		schedule    string
		timeZone    string
		expectError bool
	}{
		{"valid standard daily", "0 2 * * *", "UTC", false},
		{"valid hourly", "0 * * * *", "UTC", false},
		{"valid every minute", "* * * * *", "UTC", false},
		{"valid with timezone", "30 4 * * 1-5", "Asia/Kolkata", false},
		{"empty timezone defaults to UTC", "0 2 * * *", "", false},
		{"invalid cron format", "not-a-cron", "UTC", true},
		{"minute out of range", "60 * * * *", "UTC", true},
		{"hour out of range", "0 25 * * *", "UTC", true},
		{"invalid timezone", "0 2 * * *", "Invalid/Timezone_X", true},
		{"six fields rejected", "0 0 2 * * *", "UTC", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched, loc, err := Parse(tt.schedule, tt.timeZone)
			if (err != nil) != tt.expectError {
				t.Fatalf("Parse(%q, %q) error = %v, expectError = %v", tt.schedule, tt.timeZone, err, tt.expectError)
			}
			if !tt.expectError {
				if sched == nil || loc == nil {
					t.Fatalf("expected non-nil schedule and location")
				}
			}
		})
	}
}

func TestNextRun(t *testing.T) {
	// Base: 2026-10-24 10:00:00 UTC
	from := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)

	t.Run("daily at 2am UTC", func(t *testing.T) {
		next, err := NextRun("0 2 * * *", "UTC", from)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC)
		if !next.Equal(expected) {
			t.Errorf("NextRun = %v, expected %v", next, expected)
		}
	})

	t.Run("hourly at minute 15", func(t *testing.T) {
		next, err := NextRun("15 * * * *", "UTC", from)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := time.Date(2026, 10, 24, 10, 15, 0, 0, time.UTC)
		if !next.Equal(expected) {
			t.Errorf("NextRun = %v, expected %v", next, expected)
		}
	})

	t.Run("daily at 2am in Asia/Kolkata (+05:30)", func(t *testing.T) {
		kolkata, _ := time.LoadLocation("Asia/Kolkata")
		// 10:00 UTC is 15:30 IST. Next 02:00 IST is next day 02:00 IST (20:30 UTC previous day).
		next, err := NextRun("0 2 * * *", "Asia/Kolkata", from)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := time.Date(2026, 10, 25, 2, 0, 0, 0, kolkata)
		if !next.Equal(expected) {
			t.Errorf("NextRun = %v (%v), expected %v (%v)", next, next.In(kolkata), expected, expected.In(kolkata))
		}
	})
}

func TestLastScheduledTime(t *testing.T) {
	// Base: 2026-10-24 10:35:12 UTC
	now := time.Date(2026, 10, 24, 10, 35, 12, 0, time.UTC)

	tests := []struct {
		name        string
		schedule    string
		timeZone    string
		now         time.Time
		expected    time.Time
		expectError bool
	}{
		{
			name:     "every minute",
			schedule: "* * * * *",
			timeZone: "UTC",
			now:      now,
			expected: time.Date(2026, 10, 24, 10, 35, 0, 0, time.UTC),
		},
		{
			name:     "hourly at 00",
			schedule: "0 * * * *",
			timeZone: "UTC",
			now:      now,
			expected: time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "daily at 02:00 UTC",
			schedule: "0 2 * * *",
			timeZone: "UTC",
			now:      now,
			expected: time.Date(2026, 10, 24, 2, 0, 0, 0, time.UTC),
		},
		{
			name:     "daily at 14:00 UTC (has not happened today, should be yesterday)",
			schedule: "0 14 * * *",
			timeZone: "UTC",
			now:      now,
			expected: time.Date(2026, 10, 23, 14, 0, 0, 0, time.UTC),
		},
		{
			name:     "exact match on second 0",
			schedule: "0 10 * * *",
			timeZone: "UTC",
			now:      time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC),
			expected: time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "weekly on Sunday at midnight",
			schedule: "0 0 * * 0",
			timeZone: "UTC",
			// 2026-10-24 is a Saturday. Previous Sunday was 2026-10-18.
			now:      now,
			expected: time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			last, err := LastScheduledTime(tt.schedule, tt.timeZone, tt.now)
			if (err != nil) != tt.expectError {
				t.Fatalf("LastScheduledTime error = %v, expectError = %v", err, tt.expectError)
			}
			if !tt.expectError && !last.Equal(tt.expected) {
				t.Errorf("LastScheduledTime = %v, expected %v", last, tt.expected)
			}
		})
	}
}

func TestIsDue(t *testing.T) {
	// Schedule: daily at 02:00 UTC
	sched := "0 2 * * *"
	tz := "UTC"

	// Now: 2026-10-24 10:00:00 UTC. Last scheduled run was 2026-10-24 02:00:00 UTC.
	now := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	schedTimeToday := time.Date(2026, 10, 24, 2, 0, 0, 0, time.UTC)
	schedTimeYesterday := time.Date(2026, 10, 23, 2, 0, 0, 0, time.UTC)

	t.Run("never run before (lastRunID = 0)", func(t *testing.T) {
		due, scheduled, err := IsDue(sched, tz, 0, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !due {
			t.Errorf("expected due to be true")
		}
		if !scheduled.Equal(schedTimeToday) {
			t.Errorf("scheduled = %v, expected %v", scheduled, schedTimeToday)
		}
	})

	t.Run("last run was yesterday (catch up for today)", func(t *testing.T) {
		due, scheduled, err := IsDue(sched, tz, schedTimeYesterday.Unix(), now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !due {
			t.Errorf("expected due to be true")
		}
		if !scheduled.Equal(schedTimeToday) {
			t.Errorf("scheduled = %v, expected %v", scheduled, schedTimeToday)
		}
	})

	t.Run("last run was several days ago (multiple missed runs, only latest runs)", func(t *testing.T) {
		missedDaysAgo := time.Date(2026, 10, 20, 2, 0, 0, 0, time.UTC).Unix()
		due, scheduled, err := IsDue(sched, tz, missedDaysAgo, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !due {
			t.Errorf("expected due to be true")
		}
		if !scheduled.Equal(schedTimeToday) {
			t.Errorf("scheduled = %v, expected %v", scheduled, schedTimeToday)
		}
	})

	t.Run("already ran for today", func(t *testing.T) {
		due, scheduled, err := IsDue(sched, tz, schedTimeToday.Unix(), now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if due {
			t.Errorf("expected due to be false")
		}
		if !scheduled.Equal(schedTimeToday) {
			t.Errorf("scheduled = %v, expected %v", scheduled, schedTimeToday)
		}
	})

	t.Run("lastRunID in future", func(t *testing.T) {
		due, _, err := IsDue(sched, tz, schedTimeToday.Unix()+3600, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if due {
			t.Errorf("expected due to be false")
		}
	})
}

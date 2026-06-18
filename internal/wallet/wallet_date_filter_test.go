package wallet

import (
	"testing"
	"time"
)

// withLocalTZ pins time.Local to the given IANA zone for the duration of the
// test. matchesDateFilter's "range" branch parses ISO dates against time.Local,
// so the timezone has to be set deterministically to exercise the local-TZ
// contract under CI (which usually runs UTC).
//
// CAUTION: time.Local is a process-global *time.Location. Any test that calls
// this helper MUST NOT call t.Parallel(), and no other test in this package
// should rely on time.Local while one of these tests is running. Cleanup
// restores the original value after the test, so sequential test ordering is
// safe. If a future caller needs parallelism, refactor matchesDateFilter to
// accept an injected *time.Location instead of mutating the global.
func withLocalTZ(t *testing.T, zone string) {
	t.Helper()
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatalf("load location %q: %v", zone, err)
	}
	orig := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = orig })
}

// TestMatchesDateFilter_RangeUsesLocalTimezone pins the "range" branch of
// matchesDateFilter to local-TZ semantics. Source bug: tx whose UTC instant
// falls on the previous calendar day in UTC but on the picked day in the
// user's local TZ were excluded because time.Parse returned UTC midnight.
// Reproduced in research ?-research-tx-date-filter-timezone (Europe/Warsaw,
// 2026-05-24: "00:12 GMT+2" / "00:57 GMT+2" txs missing from From=To=24/05).
func TestMatchesDateFilter_RangeUsesLocalTimezone(t *testing.T) {
	withLocalTZ(t, "Europe/Warsaw")

	rangeFrom := "2026-05-24"
	rangeTo := "2026-05-24"

	cases := []struct {
		name    string
		txTime  time.Time
		include bool
	}{
		{
			name:    "00:12 local on May 24 (22:12 UTC May 23) included",
			txTime:  time.Date(2026, 5, 23, 22, 12, 0, 0, time.UTC),
			include: true,
		},
		{
			name:    "00:57 local on May 24 (22:57 UTC May 23) included",
			txTime:  time.Date(2026, 5, 23, 22, 57, 0, 0, time.UTC),
			include: true,
		},
		{
			name:    "07:35 local on May 24 (05:35 UTC May 24) included",
			txTime:  time.Date(2026, 5, 24, 5, 35, 0, 0, time.UTC),
			include: true,
		},
		{
			name:    "23:59 local on May 23 (21:59 UTC May 23) excluded",
			txTime:  time.Date(2026, 5, 23, 21, 59, 0, 0, time.UTC),
			include: false,
		},
		{
			name:    "00:30 local on May 25 (22:30 UTC May 24) excluded",
			txTime:  time.Date(2026, 5, 24, 22, 30, 0, 0, time.UTC),
			include: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesDateFilter(tc.txTime, "range", rangeFrom, rangeTo)
			if got != tc.include {
				t.Errorf("matchesDateFilter(%s, range, %s, %s) = %v, want %v",
					tc.txTime.Format(time.RFC3339), rangeFrom, rangeTo, got, tc.include)
			}
		})
	}
}

// TestMatchesDateFilter_RangeOnDSTSpringForward locks the contract on the
// short (23-hour) spring-forward day. Europe/Warsaw 2026 transitions at
// 2026-03-29 02:00 local → 03:00 local (loses one hour). A naive
// toDate.Add(24h - 1ns) would land the inclusive end bound at 2026-03-30
// 00:59:59.999... local instead of 2026-03-29 23:59:59.999... local,
// silently including transactions from the first hour of March 30.
// time.Date(.., day+1, 0,0,0,0, time.Local) normalizes correctly.
func TestMatchesDateFilter_RangeOnDSTSpringForward(t *testing.T) {
	withLocalTZ(t, "Europe/Warsaw")

	rangeFrom := "2026-03-29"
	rangeTo := "2026-03-29"

	cases := []struct {
		name    string
		txTime  time.Time
		include bool
	}{
		{
			// 2026-03-29 01:30 local CET (before spring-forward) = 00:30 UTC
			name:    "01:30 local on Mar 29 (before DST jump) included",
			txTime:  time.Date(2026, 3, 29, 0, 30, 0, 0, time.UTC),
			include: true,
		},
		{
			// 2026-03-29 23:30 local CEST (after spring-forward) = 21:30 UTC
			name:    "23:30 local on Mar 29 (after DST jump) included",
			txTime:  time.Date(2026, 3, 29, 21, 30, 0, 0, time.UTC),
			include: true,
		},
		{
			// 2026-03-30 00:30 local CEST = 2026-03-29 22:30 UTC.
			// With the buggy +24h - 1ns bound this would be wrongly included.
			name:    "00:30 local on Mar 30 (next day) excluded",
			txTime:  time.Date(2026, 3, 29, 22, 30, 0, 0, time.UTC),
			include: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesDateFilter(tc.txTime, "range", rangeFrom, rangeTo)
			if got != tc.include {
				t.Errorf("matchesDateFilter(%s, range, %s, %s) = %v, want %v",
					tc.txTime.Format(time.RFC3339), rangeFrom, rangeTo, got, tc.include)
			}
		})
	}
}

// TestMatchesDateFilter_RangeOnDSTFallBack locks the contract on the long
// (25-hour) fall-back day. Europe/Warsaw 2026 transitions at 2026-10-25
// 03:00 local → 02:00 local (gains one hour). A naive toDate.Add(24h - 1ns)
// would land the inclusive end bound at 2026-10-25 22:59:59.999... local
// instead of 2026-10-25 23:59:59.999... local, silently EXCLUDING
// transactions from the final hour of October 25.
func TestMatchesDateFilter_RangeOnDSTFallBack(t *testing.T) {
	withLocalTZ(t, "Europe/Warsaw")

	rangeFrom := "2026-10-25"
	rangeTo := "2026-10-25"

	cases := []struct {
		name    string
		txTime  time.Time
		include bool
	}{
		{
			// 2026-10-25 23:30 local CET (after fall-back) = 22:30 UTC.
			// With the buggy +24h - 1ns bound this would be wrongly excluded.
			name:    "23:30 local on Oct 25 (final hour after fall-back) included",
			txTime:  time.Date(2026, 10, 25, 22, 30, 0, 0, time.UTC),
			include: true,
		},
		{
			// 2026-10-26 00:30 local CET = 2026-10-25 23:30 UTC.
			name:    "00:30 local on Oct 26 (next day) excluded",
			txTime:  time.Date(2026, 10, 25, 23, 30, 0, 0, time.UTC),
			include: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesDateFilter(tc.txTime, "range", rangeFrom, rangeTo)
			if got != tc.include {
				t.Errorf("matchesDateFilter(%s, range, %s, %s) = %v, want %v",
					tc.txTime.Format(time.RFC3339), rangeFrom, rangeTo, got, tc.include)
			}
		})
	}
}

// TestMatchesDateFilter_WeekStartsOnMonday pins the "week" preset to
// ISO-week semantics (Monday is the first day of the week). The previous
// implementation used int(startOfDay.Weekday()) directly, which returns
// Sunday=0..Saturday=6 — making Sunday the first day of the week (US
// convention). That contradicted the inline calendar picker in
// TxFilterDateEditor.tsx (Mo-Tu-We-Th-Fr-Sa-Su labels) and the
// Europe/Warsaw locale.
//
// Test strategy: compute the expected Monday-aligned start-of-week from
// time.Now() using the corrected ISO formula (weekday+6)%7, then assert
// the boundary. Two assertions together fail under the buggy code
// regardless of today's weekday:
//   - tx at expectedStartOfWeek must match (catches the Sunday case
//     where buggy startOfWeek=today is AFTER corrected Monday 6d ago)
//   - tx 1ns before expectedStartOfWeek must NOT match (catches Mon-Sat
//     where buggy startOfWeek=Sunday is 1d EARLIER than corrected Monday)
func TestMatchesDateFilter_WeekStartsOnMonday(t *testing.T) {
	withLocalTZ(t, "Europe/Warsaw")

	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	isoWeekday := (int(startOfDay.Weekday()) + 6) % 7
	expectedStartOfWeek := startOfDay.AddDate(0, 0, -isoWeekday)

	cases := []struct {
		name    string
		txTime  time.Time
		include bool
	}{
		{
			name:    "tx 1 nanosecond before Monday 00:00 start of week excluded",
			txTime:  expectedStartOfWeek.Add(-time.Nanosecond),
			include: false,
		},
		{
			name:    "tx at Monday 00:00 start of week included",
			txTime:  expectedStartOfWeek,
			include: true,
		},
		{
			name:    "tx at Monday 12:00 (12h into the ISO week) included",
			txTime:  expectedStartOfWeek.Add(12 * time.Hour),
			include: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesDateFilter(tc.txTime, "week", "", "")
			if got != tc.include {
				t.Errorf("matchesDateFilter(%s, week) = %v, want %v (expected start of ISO week = %s)",
					tc.txTime.Format(time.RFC3339Nano), got, tc.include,
					expectedStartOfWeek.Format(time.RFC3339))
			}
		})
	}
}

// TestMatchesDateFilter_WeekStartsOnMonday_UTC pins the same ISO-week
// contract on a UTC daemon, so the fix is TZ-agnostic (does not rely on
// Europe/Warsaw quirks).
func TestMatchesDateFilter_WeekStartsOnMonday_UTC(t *testing.T) {
	withLocalTZ(t, "UTC")

	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	isoWeekday := (int(startOfDay.Weekday()) + 6) % 7
	expectedStartOfWeek := startOfDay.AddDate(0, 0, -isoWeekday)

	cases := []struct {
		name    string
		txTime  time.Time
		include bool
	}{
		{
			name:    "tx 1 nanosecond before Monday 00:00 UTC excluded",
			txTime:  expectedStartOfWeek.Add(-time.Nanosecond),
			include: false,
		},
		{
			name:    "tx at Monday 00:00 UTC included",
			txTime:  expectedStartOfWeek,
			include: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesDateFilter(tc.txTime, "week", "", "")
			if got != tc.include {
				t.Errorf("matchesDateFilter(%s, week) = %v, want %v",
					tc.txTime.Format(time.RFC3339Nano), got, tc.include)
			}
		})
	}
}

// TestMatchesDateFilter_LastMonthEndBoundary pins the "lastMonth" preset
// to the inclusive-start / end-exclusive-next-month contract. Acts as a
// regression net for the refactor that replaced Add(-time.Nanosecond)
// with an end-exclusive day+1 midnight bound (aligning with the "range"
// branch's DST-safe pattern). The contract itself does not change for
// non-DST month boundaries — both formulations agree at every realistic
// month transition.
func TestMatchesDateFilter_LastMonthEndBoundary(t *testing.T) {
	withLocalTZ(t, "Europe/Warsaw")

	now := time.Now()
	startOfLastMonth := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, time.Local)
	startOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)

	cases := []struct {
		name    string
		txTime  time.Time
		include bool
	}{
		{
			name:    "tx 1 nanosecond before start of last month excluded",
			txTime:  startOfLastMonth.Add(-time.Nanosecond),
			include: false,
		},
		{
			name:    "tx at start of last month 00:00 included",
			txTime:  startOfLastMonth,
			include: true,
		},
		{
			name:    "tx 1 nanosecond before start of this month included",
			txTime:  startOfThisMonth.Add(-time.Nanosecond),
			include: true,
		},
		{
			name:    "tx at start of this month 00:00 excluded",
			txTime:  startOfThisMonth,
			include: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesDateFilter(tc.txTime, "lastMonth", "", "")
			if got != tc.include {
				t.Errorf("matchesDateFilter(%s, lastMonth) = %v, want %v",
					tc.txTime.Format(time.RFC3339Nano), got, tc.include)
			}
		})
	}
}

// TestMatchesDateFilter_RangeOnUTCDaemon documents the non-regression for
// daemons running in UTC: the local-TZ fix is a no-op when time.Local == UTC,
// so existing UTC-host behavior is preserved.
func TestMatchesDateFilter_RangeOnUTCDaemon(t *testing.T) {
	withLocalTZ(t, "UTC")

	rangeFrom := "2026-05-24"
	rangeTo := "2026-05-24"

	cases := []struct {
		name    string
		txTime  time.Time
		include bool
	}{
		{
			name:    "00:00 UTC May 24 included",
			txTime:  time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
			include: true,
		},
		{
			name:    "23:59:59 UTC May 24 included",
			txTime:  time.Date(2026, 5, 24, 23, 59, 59, 0, time.UTC),
			include: true,
		},
		{
			name:    "23:59:59 UTC May 23 excluded",
			txTime:  time.Date(2026, 5, 23, 23, 59, 59, 0, time.UTC),
			include: false,
		},
		{
			name:    "00:00 UTC May 25 excluded",
			txTime:  time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC),
			include: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesDateFilter(tc.txTime, "range", rangeFrom, rangeTo)
			if got != tc.include {
				t.Errorf("matchesDateFilter(%s, range, %s, %s) = %v, want %v",
					tc.txTime.Format(time.RFC3339), rangeFrom, rangeTo, got, tc.include)
			}
		})
	}
}

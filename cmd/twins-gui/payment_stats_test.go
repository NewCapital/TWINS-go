package main

import (
	"testing"
)

// testEntries returns a set of test entries for sorting/pagination tests
func testEntries() []PaymentStatsEntry {
	return []PaymentStatsEntry{
		{Address: "DAddr3", Tier: "gold", PaymentCount: 5, TotalPaid: 100.0, LastPaidTime: "2026-03-01T10:00:00Z", LatestTxID: "tx_aaa"},
		{Address: "DAddr1", Tier: "bronze", PaymentCount: 10, TotalPaid: 50.0, LastPaidTime: "2026-03-10T10:00:00Z", LatestTxID: "tx_ccc"},
		{Address: "DAddr2", Tier: "platinum", PaymentCount: 3, TotalPaid: 200.0, LastPaidTime: "2026-02-15T10:00:00Z", LatestTxID: "tx_bbb"},
		{Address: "DAddr5", Tier: "silver", PaymentCount: 7, TotalPaid: 75.0, LastPaidTime: "2026-03-20T10:00:00Z", LatestTxID: "tx_eee"},
		{Address: "DAddr4", Tier: "", PaymentCount: 1, TotalPaid: 10.0, LastPaidTime: "", LatestTxID: "tx_ddd"},
	}
}

func TestSortByTierAsc(t *testing.T) {
	entries := testEntries()
	paymentStatsSortEntries(entries, "tier", "asc")

	// Empty string sorts first, then alphabetical: "", "bronze", "gold", "platinum", "silver"
	expected := []string{"", "bronze", "gold", "platinum", "silver"}
	for i, e := range entries {
		if e.Tier != expected[i] {
			t.Errorf("index %d: got %q, want %q", i, e.Tier, expected[i])
		}
	}
}

func TestSortByPaymentCountDesc(t *testing.T) {
	entries := testEntries()
	paymentStatsSortEntries(entries, "paymentCount", "desc")

	expected := []int64{10, 7, 5, 3, 1}
	for i, e := range entries {
		if e.PaymentCount != expected[i] {
			t.Errorf("index %d: got %d, want %d", i, e.PaymentCount, expected[i])
		}
	}
}

func TestSortByTotalPaidAsc(t *testing.T) {
	entries := testEntries()
	paymentStatsSortEntries(entries, "totalPaid", "asc")

	expected := []float64{10.0, 50.0, 75.0, 100.0, 200.0}
	for i, e := range entries {
		if e.TotalPaid != expected[i] {
			t.Errorf("index %d: got %f, want %f", i, e.TotalPaid, expected[i])
		}
	}
}

func TestSortByLastPaidTimeDesc(t *testing.T) {
	entries := testEntries()
	paymentStatsSortEntries(entries, "lastPaidTime", "desc")

	// ISO strings sort lexicographically, empty goes last in desc
	expected := []string{"2026-03-20T10:00:00Z", "2026-03-10T10:00:00Z", "2026-03-01T10:00:00Z", "2026-02-15T10:00:00Z", ""}
	for i, e := range entries {
		if e.LastPaidTime != expected[i] {
			t.Errorf("index %d: got %q, want %q", i, e.LastPaidTime, expected[i])
		}
	}
}

// TestSortUnsupportedColumnFallsBackToDefault verifies address and latestTxID
// (removed from sortable columns) fall through to default totalPaid desc.
func TestSortUnsupportedColumnFallsBackToDefault(t *testing.T) {
	for _, col := range []string{"address", "latestTxID"} {
		entries := testEntries()
		paymentStatsSortEntries(entries, col, "asc")
		// Default is always totalPaid desc regardless of direction param
		expected := []float64{200.0, 100.0, 75.0, 50.0, 10.0}
		for i, e := range entries {
			if e.TotalPaid != expected[i] {
				t.Errorf("column %q index %d: got %f, want %f", col, i, e.TotalPaid, expected[i])
			}
		}
	}
}

func TestSortDefaultColumn(t *testing.T) {
	entries := testEntries()
	// Unknown column falls back to totalPaid desc
	paymentStatsSortEntries(entries, "unknown", "asc")

	expected := []float64{200.0, 100.0, 75.0, 50.0, 10.0}
	for i, e := range entries {
		if e.TotalPaid != expected[i] {
			t.Errorf("index %d: got %f, want %f", i, e.TotalPaid, expected[i])
		}
	}
}

// TestPaginationLogic tests the pagination math used in GetPaymentStats
func TestPaginationLogic(t *testing.T) {
	tests := []struct {
		name          string
		totalEntries  int
		pageSize      int
		requestedPage int
		wantPages     int
		wantPage      int
		wantStart     int
		wantEnd       int
	}{
		{
			name: "first page of many", totalEntries: 50, pageSize: 10, requestedPage: 1,
			wantPages: 5, wantPage: 1, wantStart: 0, wantEnd: 10,
		},
		{
			name: "middle page", totalEntries: 50, pageSize: 10, requestedPage: 3,
			wantPages: 5, wantPage: 3, wantStart: 20, wantEnd: 30,
		},
		{
			name: "last page full", totalEntries: 50, pageSize: 10, requestedPage: 5,
			wantPages: 5, wantPage: 5, wantStart: 40, wantEnd: 50,
		},
		{
			name: "last page partial", totalEntries: 53, pageSize: 10, requestedPage: 6,
			wantPages: 6, wantPage: 6, wantStart: 50, wantEnd: 53,
		},
		{
			name: "page exceeds total clamped", totalEntries: 20, pageSize: 10, requestedPage: 99,
			wantPages: 2, wantPage: 2, wantStart: 10, wantEnd: 20,
		},
		{
			name: "page zero clamped to 1", totalEntries: 20, pageSize: 10, requestedPage: 0,
			wantPages: 2, wantPage: 1, wantStart: 0, wantEnd: 10,
		},
		{
			name: "negative page clamped to 1", totalEntries: 20, pageSize: 10, requestedPage: -5,
			wantPages: 2, wantPage: 1, wantStart: 0, wantEnd: 10,
		},
		{
			name: "single page", totalEntries: 5, pageSize: 25, requestedPage: 1,
			wantPages: 1, wantPage: 1, wantStart: 0, wantEnd: 5,
		},
		{
			name: "empty entries", totalEntries: 0, pageSize: 25, requestedPage: 1,
			wantPages: 1, wantPage: 1, wantStart: 0, wantEnd: 0,
		},
		{
			name: "zero pageSize defaults to 10", totalEntries: 50, pageSize: 0, requestedPage: 1,
			wantPages: 5, wantPage: 1, wantStart: 0, wantEnd: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			totalEntries := tt.totalEntries
			pageSize := tt.pageSize
			if pageSize <= 0 {
				pageSize = 10
			}
			totalPages := (totalEntries + pageSize - 1) / pageSize
			if totalPages < 1 {
				totalPages = 1
			}

			page := tt.requestedPage
			if page < 1 {
				page = 1
			}
			if page > totalPages {
				page = totalPages
			}

			start := (page - 1) * pageSize
			end := start + pageSize
			if end > totalEntries {
				end = totalEntries
			}

			if totalPages != tt.wantPages {
				t.Errorf("totalPages = %d, want %d", totalPages, tt.wantPages)
			}
			if page != tt.wantPage {
				t.Errorf("page = %d, want %d", page, tt.wantPage)
			}
			if start != tt.wantStart {
				t.Errorf("start = %d, want %d", start, tt.wantStart)
			}
			if end != tt.wantEnd {
				t.Errorf("end = %d, want %d", end, tt.wantEnd)
			}
		})
	}
}

// TestSortStability verifies that sort is stable (preserves order of equal elements)
func TestSortStability(t *testing.T) {
	entries := []PaymentStatsEntry{
		{Address: "DAddr1", Tier: "gold", PaymentCount: 5, TotalPaid: 100.0},
		{Address: "DAddr2", Tier: "gold", PaymentCount: 5, TotalPaid: 100.0},
		{Address: "DAddr3", Tier: "gold", PaymentCount: 5, TotalPaid: 100.0},
	}

	paymentStatsSortEntries(entries, "tier", "asc")

	// All have same tier, so original order should be preserved
	expected := []string{"DAddr1", "DAddr2", "DAddr3"}
	for i, e := range entries {
		if e.Address != expected[i] {
			t.Errorf("stable sort failed at index %d: got %s, want %s", i, e.Address, expected[i])
		}
	}
}

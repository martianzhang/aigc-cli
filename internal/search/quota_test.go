package search

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// openTestQuotaStore opens a fresh in-memory SQLite quota store.
func openTestQuotaStore(t *testing.T) *SQLiteQuotaStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	// database/sql pools connections, but each connection to ":memory:"
	// gets its own database. Pin to one connection so the migrated table
	// survives for the whole test.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { db.Close() })

	store, err := NewQuotaStore(db)
	if err != nil {
		t.Fatalf("NewQuotaStore: %v", err)
	}
	return store
}

// seedQuota inserts a quota record directly, bypassing RecordUse.
func seedQuota(t *testing.T, s *SQLiteQuotaStore, provider string, used, total int, period, periodStart string) {
	t.Helper()
	_, err := s.db.Exec(`
		INSERT INTO search_quota (provider, used, total, period, period_start)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(provider) DO UPDATE SET
			used = excluded.used,
			total = excluded.total,
			period = excluded.period,
			period_start = excluded.period_start
	`, provider, used, total, period, periodStart)
	if err != nil {
		t.Fatalf("seed quota for %q: %v", provider, err)
	}
}

// rfc3339Ago returns an RFC3339 timestamp d before now (UTC).
func rfc3339Ago(d time.Duration) string {
	return time.Now().UTC().Add(-d).Format(time.RFC3339)
}

func TestShouldReset_hourly(t *testing.T) {
	if !shouldReset("hourly", rfc3339Ago(2*time.Hour)) {
		t.Error("hourly period started 2h ago should reset")
	}
}

func TestShouldReset_daily(t *testing.T) {
	if !shouldReset("daily", rfc3339Ago(48*time.Hour)) {
		t.Error("daily period started 48h ago should reset")
	}
}

func TestShouldReset_monthly(t *testing.T) {
	if !shouldReset("monthly", rfc3339Ago(31*24*time.Hour)) {
		t.Error("monthly period started 31d ago should reset")
	}
}

func TestShouldReset_withinPeriod(t *testing.T) {
	tests := []struct {
		name       string
		period     string
		startedAgo time.Duration
	}{
		{"hourly within period", "hourly", 30 * time.Minute},
		{"daily within period", "daily", 23 * time.Hour},
		{"monthly within period", "monthly", 29 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if shouldReset(tt.period, rfc3339Ago(tt.startedAgo)) {
				t.Errorf("%s period started %v ago should not reset", tt.period, tt.startedAgo)
			}
		})
	}
}

func TestShouldReset_unknownPeriod(t *testing.T) {
	for _, period := range []string{"", "weekly", "yearly"} {
		if shouldReset(period, rfc3339Ago(1000*time.Hour)) {
			t.Errorf("unknown period %q should not reset", period)
		}
	}
}

func TestShouldReset_parseError(t *testing.T) {
	for _, periodStart := range []string{"", "not-a-timestamp", "2024-01-01 12:00:00"} {
		if !shouldReset("hourly", periodStart) {
			t.Errorf("unparseable periodStart %q should reset", periodStart)
		}
	}
}

func TestSQLiteQuotaStore_CanUse(t *testing.T) {
	tests := []struct {
		name  string
		seed  bool
		used  int
		total int
		want  bool
	}{
		{"no record is unlimited", false, 0, 0, true},
		{"total zero is unlimited", true, 5, 0, true},
		{"used below total", true, 3, 10, true},
		{"used equals total", true, 10, 10, false},
		{"used above total", true, 11, 10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := openTestQuotaStore(t)
			if tt.seed {
				seedQuota(t, s, "duckduckgo", tt.used, tt.total, "daily", rfc3339Ago(time.Minute))
			}
			if got := s.CanUse("duckduckgo"); got != tt.want {
				t.Errorf("CanUse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSQLiteQuotaStore_RecordUse(t *testing.T) {
	t.Run("creates record on first use", func(t *testing.T) {
		s := openTestQuotaStore(t)
		s.RecordUse("firecrawl")
		if used, total := s.Usage("firecrawl"); used != 1 || total != 0 {
			t.Errorf("after first RecordUse: used=%d total=%d, want 1 0", used, total)
		}
	})

	t.Run("increments existing counter and keeps total", func(t *testing.T) {
		s := openTestQuotaStore(t)
		seedQuota(t, s, "firecrawl", 2, 10, "daily", rfc3339Ago(time.Minute))
		s.RecordUse("firecrawl")
		if used, total := s.Usage("firecrawl"); used != 3 || total != 10 {
			t.Errorf("after RecordUse: used=%d total=%d, want 3 10", used, total)
		}
	})
}

func TestSQLiteQuotaStore_Usage(t *testing.T) {
	s := openTestQuotaStore(t)
	seedQuota(t, s, "brave", 4, 100, "monthly", rfc3339Ago(time.Hour))

	used, total := s.Usage("brave")
	if used != 4 || total != 100 {
		t.Errorf("Usage(brave) = %d, %d, want 4, 100", used, total)
	}

	if used, total := s.Usage("missing"); used != 0 || total != 0 {
		t.Errorf("Usage(missing) = %d, %d, want 0, 0", used, total)
	}
}

func TestSQLiteQuotaStore_ResetIfNeeded(t *testing.T) {
	t.Run("resets when period elapsed", func(t *testing.T) {
		s := openTestQuotaStore(t)
		seedQuota(t, s, "brave", 5, 10, "daily", rfc3339Ago(48*time.Hour))

		s.ResetIfNeeded("brave")

		if used, total := s.Usage("brave"); used != 0 || total != 10 {
			t.Errorf("after reset: used=%d total=%d, want 0 10", used, total)
		}

		var periodStart string
		if err := s.db.QueryRow("SELECT period_start FROM search_quota WHERE provider=?", "brave").Scan(&periodStart); err != nil {
			t.Fatalf("query period_start: %v", err)
		}
		start, err := time.Parse(time.RFC3339, periodStart)
		if err != nil {
			t.Fatalf("parse refreshed period_start %q: %v", periodStart, err)
		}
		if time.Since(start) > time.Minute {
			t.Errorf("period_start not refreshed after reset: %s", periodStart)
		}
	})

	t.Run("keeps usage within period", func(t *testing.T) {
		s := openTestQuotaStore(t)
		seedQuota(t, s, "brave", 5, 10, "daily", rfc3339Ago(30*time.Minute))

		s.ResetIfNeeded("brave")

		if used, total := s.Usage("brave"); used != 5 || total != 10 {
			t.Errorf("usage reset within period: used=%d total=%d, want 5 10", used, total)
		}
	})

	t.Run("no record is a no-op", func(t *testing.T) {
		s := openTestQuotaStore(t)
		s.ResetIfNeeded("unknown")
		if used, total := s.Usage("unknown"); used != 0 || total != 0 {
			t.Errorf("Usage(unknown) = %d, %d, want 0, 0", used, total)
		}
	})
}

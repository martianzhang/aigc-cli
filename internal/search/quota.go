package search

import (
	"database/sql"
	"time"
)

// SQLiteQuotaStore tracks search provider usage via SQLite.
type SQLiteQuotaStore struct {
	db *sql.DB
}

// NewQuotaStore creates a quota store using the given DB.
// Creates the quota table if it doesn't exist.
func NewQuotaStore(db *sql.DB) (*SQLiteQuotaStore, error) {
	s := &SQLiteQuotaStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *SQLiteQuotaStore) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS search_quota (
			provider     TEXT PRIMARY KEY,
			used         INTEGER DEFAULT 0,
			total        INTEGER DEFAULT 0,
			period       TEXT DEFAULT '',
			period_start TEXT DEFAULT ''
		)
	`)
	return err
}

// CanUse returns true if the provider has quota remaining.
func (s *SQLiteQuotaStore) CanUse(provider string) bool {
	s.ResetIfNeeded(provider)
	var used, total int
	err := s.db.QueryRow("SELECT used, total FROM search_quota WHERE provider=?", provider).Scan(&used, &total)
	if err != nil {
		return true // no quota record = unlimited
	}
	if total <= 0 {
		return true // unlimited
	}
	return used < total
}

// RecordUse increments the usage counter for a provider.
func (s *SQLiteQuotaStore) RecordUse(provider string) {
	s.db.Exec(`
		INSERT INTO search_quota (provider, used, total, period, period_start)
		VALUES (?, 1, 0, '', ?)
		ON CONFLICT(provider) DO UPDATE SET used = used + 1
	`, provider, time.Now().UTC().Format(time.RFC3339))
}

// ResetIfNeeded resets the usage counter if the period has elapsed.
func (s *SQLiteQuotaStore) ResetIfNeeded(provider string) {
	var period, periodStart string
	err := s.db.QueryRow(
		"SELECT period, period_start FROM search_quota WHERE provider=?", provider,
	).Scan(&period, &periodStart)
	if err != nil || period == "" || periodStart == "" {
		return
	}
	if shouldReset(period, periodStart) {
		s.db.Exec("UPDATE search_quota SET used=0, period_start=? WHERE provider=?",
			time.Now().UTC().Format(time.RFC3339), provider)
	}
}

// Usage returns the current used and total quota for a provider.
func (s *SQLiteQuotaStore) Usage(provider string) (int, int) {
	var used, total int
	err := s.db.QueryRow(
		"SELECT used, total FROM search_quota WHERE provider=?", provider,
	).Scan(&used, &total)
	if err != nil {
		return 0, 0
	}
	return used, total
}

func shouldReset(period, periodStart string) bool {
	start, err := time.Parse(time.RFC3339, periodStart)
	if err != nil {
		return true
	}
	now := time.Now().UTC()
	switch period {
	case "hourly":
		return now.Sub(start) > time.Hour
	case "daily":
		return now.Sub(start) > 24*time.Hour
	case "monthly":
		return now.Sub(start) > 30*24*time.Hour
	default:
		return false
	}
}

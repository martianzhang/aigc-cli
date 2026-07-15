package ideas

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const (
	ideasDirName  = ".config/aigc-cli/ideas"
	schemaVersion = "1"
	dbFileName    = "ideas.db"

	sqlCreateMeta = `CREATE TABLE IF NOT EXISTS meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`

	sqlCreateIdeas = `CREATE TABLE IF NOT EXISTS ideas (
		id         INTEGER PRIMARY KEY,
		title      TEXT NOT NULL DEFAULT '',
		title_zh   TEXT NOT NULL DEFAULT '',
		prompt     TEXT NOT NULL,
		prompt_zh  TEXT NOT NULL DEFAULT '',
		image_urls TEXT NOT NULL DEFAULT '[]',
		source_url TEXT NOT NULL DEFAULT '',
		author     TEXT NOT NULL DEFAULT '',
		license    TEXT NOT NULL DEFAULT '',
		lang       TEXT NOT NULL DEFAULT ''
	);`

	sqlCreateInvertedIndex = `CREATE TABLE IF NOT EXISTS inverted_index (
		term     TEXT    NOT NULL,
		entry_id INTEGER NOT NULL,
		tf       INTEGER NOT NULL,
		PRIMARY KEY (term, entry_id)
	) WITHOUT ROWID;`

	sqlCreateInvertedIdx = `CREATE INDEX IF NOT EXISTS idx_inverted_entry ON inverted_index(entry_id);`
)

// defaultDBPath returns the default path to the ideas SQLite database.
func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ideasDirName, dbFileName)
}

// DBPathFromJSON returns the DB path corresponding to a JSON data path.
// If jsonPath is empty, returns the default DB path.
func DBPathFromJSON(jsonPath string) string {
	if jsonPath != "" {
		return strings.TrimSuffix(jsonPath, ".json") + ".db"
	}
	return defaultDBPath()
}

// OpenDB opens the ideas SQLite database at the given path.
// If dbPath is empty, uses the default location.
// If the database doesn't exist or is outdated, it automatically imports
// from the corresponding ideas.json (same path, .json extension).
func OpenDB(dbPath string) (*sql.DB, error) {
	path := dbPath
	if path == "" {
		path = defaultDBPath()
	}
	if path == "" {
		return nil, fmt.Errorf("cannot determine ideas database path")
	}

	// Auto-import from ideas.json if the database is missing or empty.
	if err := ensureDB(path); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open ideas database %s: %w", path, err)
	}
	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}
	return db, nil
}

// ensureDB checks if the database at dbPath is valid and up-to-date.
// If the database doesn't exist or has no data, it looks for the corresponding
// ideas.json (derived from dbPath), and imports it automatically.
// Returns nil if the database is ready (either already existed or was just imported).
func ensureDB(dbPath string) error {
	// Quick check: DB file exists and has meta table.
	if fileExists(dbPath) {
		db, err := sql.Open("sqlite", dbPath)
		if err == nil {
			defer db.Close()
			var n int
			if err := db.QueryRow("SELECT COUNT(*) FROM meta").Scan(&n); err == nil && n > 0 {
				return nil // DB is valid, nothing to do.
			}
		}
	}

	// DB is missing or empty. Try to import from local ideas.json.
	jsonPath := strings.TrimSuffix(dbPath, ".db") + ".json"
	data, err := os.ReadFile(filepath.Clean(jsonPath))
	if err != nil {
		return fmt.Errorf("ideas database not found at %s and cannot read ideas.json at %s\n  Run 'aigc-cli ideas init' to set up the dataset", dbPath, jsonPath)
	}

	hash := SourceHash(data)
	entries, err := decodeEntries(data)
	if err != nil {
		return fmt.Errorf("ideas.json at %s is corrupted: %w", jsonPath, err)
	}

	if err := InitDB(dbPath, entries, hash); err != nil {
		return fmt.Errorf("failed to build search index from %s: %w", jsonPath, err)
	}
	return nil
}

// decodeEntries validates and unmarshals ideas.json bytes into IdeaEntry slice.
func decodeEntries(data []byte) ([]IdeaEntry, error) {
	var entries []IdeaEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// fileExists reports whether the named file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// InitDB creates the ideas database schema and populates it with entries.
// The database file is created at dbPath (or default location if empty).
func InitDB(dbPath string, entries []IdeaEntry, sourceHash string) error {
	path := dbPath
	if path == "" {
		path = defaultDBPath()
	}
	if path == "" {
		return fmt.Errorf("cannot determine ideas database path")
	}

	// Remove existing DB file to start fresh.
	os.Remove(path)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", dir, err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("failed to create database %s: %w", path, err)
	}
	defer db.Close()

	// Enable WAL mode.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("failed to set WAL mode: %w", err)
	}

	// Begin transaction for bulk import.
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Create tables.
	for _, ddl := range []string{sqlCreateMeta, sqlCreateIdeas, sqlCreateInvertedIndex, sqlCreateInvertedIdx} {
		if _, err := tx.Exec(ddl); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	// Prepare statements for ideas insert and inverted index insert.
	stmtIdea, err := tx.Prepare(`INSERT INTO ideas (id, title, title_zh, prompt, prompt_zh, image_urls, source_url, author, license, lang) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare idea insert: %w", err)
	}
	defer stmtIdea.Close()

	stmtIndex, err := tx.Prepare(`INSERT INTO inverted_index (term, entry_id, tf) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare index insert: %w", err)
	}
	defer stmtIndex.Close()

	// Insert entries and build inverted index.
	totalTokens := 0
	for i, e := range entries {
		id := i + 1 // 1-based ID
		imgURLs, _ := json.Marshal(e.ImageURLs)
		if _, err := stmtIdea.Exec(id, e.Title, e.TitleZh, e.Prompt, e.PromptZh, string(imgURLs), e.SourceURL, e.Author, e.License, e.Lang); err != nil {
			return fmt.Errorf("failed to insert idea %d: %w", id, err)
		}

		// Tokenize and build inverted index.
		text := searchableText(e)
		terms := tokenize(text)
		totalTokens += len(terms)
		tf := make(map[string]int, len(terms)/2)
		for _, t := range terms {
			tf[t]++
		}
		for term, freq := range tf {
			if _, err := stmtIndex.Exec(term, id, freq); err != nil {
				return fmt.Errorf("failed to insert inverted index for entry %d, term %q: %w", id, term, err)
			}
		}
	}

	// Write meta data.
	meta := map[string]string{
		"source_hash":    sourceHash,
		"schema_version": schemaVersion,
		"total_docs":     fmt.Sprintf("%d", len(entries)),
		"avg_doc_len":    fmt.Sprintf("%.2f", float64(totalTokens)/float64(max(len(entries), 1))),
	}
	for k, v := range meta {
		if _, err := tx.Exec("INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)", k, v); err != nil {
			return fmt.Errorf("failed to insert meta %s: %w", k, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// SourceHash computes the SHA256 hash of the given data.
func SourceHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// GetSourceHash reads the source_hash from the database meta table.
func GetSourceHash(db *sql.DB) (string, error) {
	var hash string
	err := db.QueryRow("SELECT value FROM meta WHERE key = 'source_hash'").Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to read source_hash: %w", err)
	}
	return hash, nil
}

// CheckSourceHash checks if the database was built from the given source hash.
// Returns true if the hash matches (no rebuild needed).
func CheckSourceHash(db *sql.DB, sourceHash string) (bool, error) {
	stored, err := GetSourceHash(db)
	if err != nil {
		return false, err
	}
	return stored == sourceHash, nil
}

// IdeaFromRow scans a single ideas row into an IdeaEntry.
func IdeaFromRow(row *sql.Rows) (IdeaEntry, error) {
	var e IdeaEntry
	var imgURLs string
	if err := row.Scan(&e.Title, &e.TitleZh, &e.Prompt, &e.PromptZh, &imgURLs, &e.SourceURL, &e.Author, &e.License, &e.Lang); err != nil {
		return e, err
	}
	if imgURLs != "" {
		json.Unmarshal([]byte(imgURLs), &e.ImageURLs)
	}
	return e, nil
}

// QueryCandidateIDs returns entry IDs that match ALL query terms (AND filter).
// Uses the inverted index for fast intersection.
func QueryCandidateIDs(db *sql.DB, queryTerms []string) ([]int, error) {
	if len(queryTerms) == 0 {
		return nil, nil
	}
	if len(queryTerms) == 1 {
		rows, err := db.Query("SELECT entry_id FROM inverted_index WHERE term = ?", queryTerms[0])
		if err != nil {
			return nil, fmt.Errorf("failed to query inverted index: %w", err)
		}
		defer rows.Close()
		var ids []int
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	}

	// Multiple terms: INTERSECT.
	query := "SELECT entry_id FROM inverted_index WHERE term = ?"
	for i := 1; i < len(queryTerms); i++ {
		query += " INTERSECT SELECT entry_id FROM inverted_index WHERE term = ?"
	}
	args := make([]interface{}, len(queryTerms))
	for i, t := range queryTerms {
		args[i] = t
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to intersect inverted index: %w", err)
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// LoadEntries loads full IdeaEntry data by their IDs.
func LoadEntries(db *sql.DB, ids []int) ([]IdeaEntry, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	// Build IN clause with placeholders.
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := "SELECT title, title_zh, prompt, prompt_zh, image_urls, source_url, author, license, lang FROM ideas WHERE id IN (" + strings.Join(placeholders, ",") + ")"
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to load entries: %w", err)
	}
	defer rows.Close()

	var entries []IdeaEntry
	for rows.Next() {
		e, err := IdeaFromRow(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// LoadRandomEntries returns a random selection of entries.
func LoadRandomEntries(db *sql.DB, limit int) ([]IdeaEntry, error) {
	rows, err := db.Query("SELECT title, title_zh, prompt, prompt_zh, image_urls, source_url, author, license, lang FROM ideas ORDER BY RANDOM() LIMIT ?", limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query random entries: %w", err)
	}
	defer rows.Close()

	var entries []IdeaEntry
	for rows.Next() {
		e, err := IdeaFromRow(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// SearchEntriesByImage finds entries whose image_urls contain the given filename.
func SearchEntriesByImage(db *sql.DB, filename string) ([]IdeaEntry, error) {
	fn := strings.ToLower(filename)
	rows, err := db.Query("SELECT title, title_zh, prompt, prompt_zh, image_urls, source_url, author, license, lang FROM ideas WHERE LOWER(image_urls) LIKE ?", "%"+fn+"%")
	if err != nil {
		return nil, fmt.Errorf("failed to search by image: %w", err)
	}
	defer rows.Close()

	var entries []IdeaEntry
	seen := make(map[string]bool)
	for rows.Next() {
		e, err := IdeaFromRow(rows)
		if err != nil {
			return nil, err
		}
		// Deduplicate by image URLs.
		key := strings.Join(e.ImageURLs, "|")
		if key == "" {
			key = e.SourceURL
		}
		if key == "" {
			key = e.Title + "|" + e.Prompt
		}
		if !seen[key] {
			seen[key] = true
			entries = append(entries, e)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// GetCorpusStats returns the corpus-level statistics needed for BM25 IDF calculation.
func GetCorpusStats(db *sql.DB) (totalDocs int, avgDocLen float64, err error) {
	var td string
	var adl string
	if e := db.QueryRow("SELECT value FROM meta WHERE key = 'total_docs'").Scan(&td); e != nil {
		return 0, 0, e
	}
	if e := db.QueryRow("SELECT value FROM meta WHERE key = 'avg_doc_len'").Scan(&adl); e != nil {
		return 0, 0, e
	}
	fmt.Sscanf(td, "%d", &totalDocs)
	fmt.Sscanf(adl, "%f", &avgDocLen)
	return totalDocs, avgDocLen, nil
}

// GetTermDocFreq returns the document frequency (number of docs containing the term)
// and collects entry_id→tf pairs for BM25 scoring.
func GetTermDocFreq(db *sql.DB, term string) (df int, entryTF map[int]int, err error) {
	rows, err := db.Query("SELECT entry_id, tf FROM inverted_index WHERE term = ?", term)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to query term %q: %w", term, err)
	}
	defer rows.Close()

	entryTF = make(map[int]int)
	for rows.Next() {
		var id, tf int
		if err := rows.Scan(&id, &tf); err != nil {
			return 0, nil, err
		}
		entryTF[id] = tf
		df++
	}
	if err := rows.Err(); err != nil {
		return 0, nil, err
	}
	return df, entryTF, nil
}

// DBExists checks if ideas database exists at the given path (or default).
func DBExists(dbPath string) bool {
	path := dbPath
	if path == "" {
		path = defaultDBPath()
	}
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// LoadAllEntries loads all ideas from the database.
// Used for operations that need the full dataset (SearchByImage without DB access).
func LoadAllEntries(db *sql.DB) ([]IdeaEntry, error) {
	rows, err := db.Query("SELECT title, title_zh, prompt, prompt_zh, image_urls, source_url, author, license, lang FROM ideas ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("failed to load all entries: %w", err)
	}
	defer rows.Close()

	var entries []IdeaEntry
	for rows.Next() {
		e, err := IdeaFromRow(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

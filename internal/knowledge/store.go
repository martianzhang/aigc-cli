package knowledge

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"path/filepath"
	"time"

	"modernc.org/sqlite"
)

// Store wraps the SQLite database for the knowledge base.
type Store struct {
	db       *sql.DB
	dim      int
	embedder Embedder // optional ONNX embedder; nil = use HashEmbedder
	minScore float64  // minimum score threshold for vector results (default 0.8)
}

// OpenStore opens (or creates) the knowledge base database.
// If embedder is nil, HashEmbedder is used for vector search.
func OpenStore(baseDir string, dim int, embedder Embedder) (*Store, error) {
	dbPath := filepath.Join(baseDir, "knowledge.db")
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	s := &Store{db: db, dim: dim, embedder: embedder, minScore: 0.8}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate store: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// SetMinScore sets the minimum score threshold for search results.
// Results with score below this value are filtered out. 0 = no filter.
func (s *Store) SetMinScore(v float64) { s.minScore = v }

// DB returns the underlying *sql.DB for advanced queries.
func (s *Store) DB() *sql.DB { return s.db }

// schema creates all tables and indexes.
const schema = `
CREATE TABLE IF NOT EXISTS documents (
    id          TEXT PRIMARY KEY,
    url         TEXT DEFAULT '',
    filepath    TEXT DEFAULT '',
    title       TEXT DEFAULT '',
    size        INTEGER DEFAULT 0,
    checksum    TEXT DEFAULT '',
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
    content
);

CREATE TABLE IF NOT EXISTS chunks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    doc_id      TEXT NOT NULL REFERENCES documents(id),
    chunk_index INTEGER DEFAULT 0,
    content     TEXT NOT NULL,
    heading     TEXT DEFAULT '',
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS embeddings (
    doc_id      TEXT NOT NULL REFERENCES documents(id),
    chunk_id    INTEGER NOT NULL REFERENCES chunks(id),
    vector      BLOB NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_chunks_doc_id ON chunks(doc_id);
CREATE INDEX IF NOT EXISTS idx_embeddings_doc_id ON embeddings(doc_id);
CREATE INDEX IF NOT EXISTS idx_embeddings_chunk_id ON embeddings(chunk_id);

CREATE TABLE IF NOT EXISTS search_quota (
    provider    TEXT PRIMARY KEY,
    used        INTEGER DEFAULT 0,
    total       INTEGER,
    period      TEXT,
    period_start TEXT
);
`

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// Migration: add project column for existing databases
	_, err := s.db.Exec("ALTER TABLE documents ADD COLUMN project TEXT DEFAULT ''")
	if err != nil {
		_ = err
	}
	// Migration: add is_vault column
	_, err = s.db.Exec("ALTER TABLE documents ADD COLUMN is_vault INTEGER DEFAULT 0")
	if err != nil {
		_ = err
	}
	return nil
}

// SaveDocument inserts or updates a document.
func (s *Store) SaveDocument(doc *Document) error {
	_, err := s.db.Exec(`
		INSERT INTO documents (id, url, filepath, title, project, size, checksum, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			url=excluded.url, filepath=excluded.filepath,
			title=excluded.title, project=excluded.project,
			size=excluded.size, checksum=excluded.checksum,
			updated_at=CURRENT_TIMESTAMP`,
		doc.ID, doc.URL, doc.FilePath, doc.Title, doc.Project, doc.Size, doc.Checksum)
	return err
}

// GetDocument retrieves a document by ID.
func (s *Store) GetDocument(id string) (*Document, error) {
	row := s.db.QueryRow(`
		SELECT id, COALESCE(url,''), COALESCE(filepath,''),
		       COALESCE(title,''), COALESCE(project,''),
		       COALESCE(size,0), COALESCE(checksum,''),
		       created_at, updated_at
		FROM documents WHERE id=?`, id)
	doc := &Document{}
	err := row.Scan(&doc.ID, &doc.URL, &doc.FilePath, &doc.Title,
		&doc.Project, &doc.Size, &doc.Checksum, &doc.CreatedAt, &doc.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return doc, err
}

// ListDocuments returns all documents ordered by updated_at desc.
// If project is non-empty, filters to that project.
func (s *Store) ListDocuments(limit, offset int, project string) ([]Document, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var rows *sql.Rows
	var err error
	if project != "" {
		rows, err = s.db.Query(`
			SELECT id, COALESCE(url,''), COALESCE(filepath,''),
			       COALESCE(title,''), COALESCE(project,''),
			       COALESCE(size,0), COALESCE(checksum,''),
			       created_at, updated_at
			FROM documents WHERE project=? ORDER BY updated_at DESC LIMIT ? OFFSET ?`, project, limit, offset)
	} else {
		rows, err = s.db.Query(`
			SELECT id, COALESCE(url,''), COALESCE(filepath,''),
			       COALESCE(title,''), COALESCE(project,''),
			       COALESCE(size,0), COALESCE(checksum,''),
			       created_at, updated_at
			FROM documents ORDER BY updated_at DESC LIMIT ? OFFSET ?`, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var docs []Document
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.URL, &d.FilePath, &d.Title,
			&d.Project, &d.Size, &d.Checksum, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// DeleteDocument removes a document and its chunks and embeddings.
func (s *Store) DeleteDocument(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete embeddings
	if _, err := tx.Exec("DELETE FROM embeddings WHERE doc_id=?", id); err != nil {
		return err
	}
	// Delete chunks
	if _, err := tx.Exec("DELETE FROM chunks WHERE doc_id=?", id); err != nil {
		return err
	}
	// Delete document
	if _, err := tx.Exec("DELETE FROM documents WHERE id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// CountDocuments returns the number of documents.
func (s *Store) CountDocuments() (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM documents").Scan(&n)
	return n, err
}

// SaveChunks saves chunks for a document with their embeddings.
// When skipFTS is true, the FTS5 index is not updated (for vault docs).
func (s *Store) SaveChunks(docID string, chunks []Chunk, embeddings []Embedding, skipFTS bool) error {
	if len(chunks) != len(embeddings) {
		return fmt.Errorf("chunks/embeddings count mismatch: %d vs %d", len(chunks), len(embeddings))
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Collect old chunk IDs for this doc before deleting
	oldRows, err := tx.Query("SELECT id FROM chunks WHERE doc_id=?", docID)
	if err != nil {
		return err
	}
	var oldChunkIDs []int64
	for oldRows.Next() {
		var id int64
		if err := oldRows.Scan(&id); err == nil {
			oldChunkIDs = append(oldChunkIDs, id)
		}
	}
	oldRows.Close()

	// Delete old chunks/embeddings for this doc
	if _, err := tx.Exec("DELETE FROM embeddings WHERE doc_id=?", docID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM chunks WHERE doc_id=?", docID); err != nil {
		return err
	}
	// Delete FTS entries for old chunks by rowid
	for _, id := range oldChunkIDs {
		tx.Exec("DELETE FROM chunks_fts WHERE rowid=?", id)
	}

	// Insert chunks and embeddings
	chunkStmt, err := tx.Prepare("INSERT INTO chunks (doc_id, chunk_index, content, heading) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer chunkStmt.Close()

	var ftsStmt *sql.Stmt
	if !skipFTS {
		ftsStmt, err = tx.Prepare("INSERT INTO chunks_fts (rowid, content) VALUES (?, ?)")
		if err != nil {
			return err
		}
		defer ftsStmt.Close()
	}

	embedStmt, err := tx.Prepare("INSERT INTO embeddings (doc_id, chunk_id, vector) VALUES (?, ?, ?)")
	if err != nil {
		return err
	}
	defer embedStmt.Close()

	for i := range chunks {
		res, err := chunkStmt.Exec(docID, chunks[i].Index, chunks[i].Content, chunks[i].Heading)
		if err != nil {
			return fmt.Errorf("insert chunk %d: %w", i, err)
		}
		chunkID, err := res.LastInsertId()
		if err != nil {
			return err
		}

		// FTS (skip for vault docs)
		if ftsStmt != nil {
			if _, err := ftsStmt.Exec(chunkID, chunks[i].Content); err != nil {
				if isFTS5Duplicate(err) {
					continue
				}
				return fmt.Errorf("insert FTS %d: %w", i, err)
			}
		}

		// Embedding
		vecBlob := embeddingToBlob(embeddings[i])
		if _, err := embedStmt.Exec(docID, chunkID, vecBlob); err != nil {
			return fmt.Errorf("insert embedding %d: %w", i, err)
		}
	}

	return tx.Commit()
}

// SaveVaultEmbeddings stores embedding vectors for vault docs without plaintext.
func (s *Store) SaveVaultEmbeddings(docID string, embeddings []Embedding) error {
	if len(embeddings) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM embeddings WHERE doc_id=?", docID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM chunks WHERE doc_id=?", docID); err != nil {
		return err
	}
	cs, err := tx.Prepare("INSERT INTO chunks (doc_id, chunk_index, content, heading) VALUES (?, ?, '', '')")
	if err != nil {
		return err
	}
	defer cs.Close()
	es, err := tx.Prepare("INSERT INTO embeddings (doc_id, chunk_id, vector) VALUES (?, ?, ?)")
	if err != nil {
		return err
	}
	defer es.Close()
	for i := range embeddings {
		res, err := cs.Exec(docID, i)
		if err != nil {
			return fmt.Errorf("chunk %d: %w", i, err)
		}
		cid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		blob := embeddingToBlob(embeddings[i])
		if _, err := es.Exec(docID, cid, blob); err != nil {
			return fmt.Errorf("embed %d: %w", i, err)
		}
	}
	return tx.Commit()
}

// SearchFTS performs a full-text search via FTS5, optionally filtered by project.
// When project is non-empty, limits results to documents with that project.
// When project is "global" (literal), limits to documents with project = ”.
// When project is empty string, returns results from all projects.
func (s *Store) SearchFTS(query string, limit int, project string) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows *sql.Rows
	var err error
	if project == "" {
		rows, err = s.db.Query(`
			SELECT c.id, c.doc_id, c.chunk_index, c.content, c.heading, c.created_at,
			       d.id, COALESCE(d.url,''), COALESCE(d.filepath,''),
			       COALESCE(d.title,''), COALESCE(d.project,''), d.size, d.created_at, d.updated_at,
			       rank
			FROM chunks_fts f
			JOIN chunks c ON f.rowid = c.id
			JOIN documents d ON c.doc_id = d.id
			WHERE chunks_fts MATCH ?
			ORDER BY rank
			LIMIT ?`, query, limit)
	} else {
		projectVal := project
		if projectVal == "global" {
			projectVal = ""
		}
		rows, err = s.db.Query(`
			SELECT c.id, c.doc_id, c.chunk_index, c.content, c.heading, c.created_at,
			       d.id, COALESCE(d.url,''), COALESCE(d.filepath,''),
			       COALESCE(d.title,''), COALESCE(d.project,''), d.size, d.created_at, d.updated_at,
			       rank
			FROM chunks_fts f
			JOIN chunks c ON f.rowid = c.id
			JOIN documents d ON c.doc_id = d.id
			WHERE chunks_fts MATCH ? AND d.project = ?
			ORDER BY rank
			LIMIT ?`, query, projectVal, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResults(rows)
}

// SearchVector performs brute-force cosine similarity search over all embeddings,
// optionally filtered by project.
func (s *Store) SearchVector(target Embedding, limit int, project string) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows *sql.Rows
	var err error
	if project == "" {
		rows, err = s.db.Query(`
			SELECT c.id, c.doc_id, c.chunk_index, c.content, c.heading, c.created_at,
			       d.id, COALESCE(d.url,''), COALESCE(d.filepath,''),
			       COALESCE(d.title,''), COALESCE(d.project,''), d.size, d.created_at, d.updated_at,
			       0.0
			FROM embeddings e
			JOIN chunks c ON e.chunk_id = c.id
			JOIN documents d ON e.doc_id = d.id
			ORDER BY cosine_similarity(e.vector, ?) DESC
			LIMIT ?`, embeddingToBlob(target), limit)
	} else {
		projectVal := project
		if projectVal == "global" {
			projectVal = ""
		}
		rows, err = s.db.Query(`
			SELECT c.id, c.doc_id, c.chunk_index, c.content, c.heading, c.created_at,
			       d.id, COALESCE(d.url,''), COALESCE(d.filepath,''),
			       COALESCE(d.title,''), COALESCE(d.project,''), d.size, d.created_at, d.updated_at,
			       0.0
			FROM embeddings e
			JOIN chunks c ON e.chunk_id = c.id
			JOIN documents d ON e.doc_id = d.id
			WHERE d.project = ?
			ORDER BY cosine_similarity(e.vector, ?) DESC
			LIMIT ?`, projectVal, embeddingToBlob(target), limit)
	}
	if err != nil {
		return s.searchVectorGo(target, limit, project)
	}
	defer rows.Close()
	return scanResults(rows)
}

// searchVectorGo performs brute-force cosine similarity in Go.
func (s *Store) searchVectorGo(target Embedding, limit int, project string) ([]SearchResult, error) {
	rows, err := s.db.Query(`
		SELECT e.vector, c.id, c.doc_id, c.chunk_index, c.content, c.heading, c.created_at,
		       d.id, COALESCE(d.url,''), COALESCE(d.filepath,''),
		       COALESCE(d.title,''), COALESCE(d.project,''), d.size, d.created_at, d.updated_at
		FROM embeddings e
		JOIN chunks c ON e.chunk_id = c.id
		JOIN documents d ON e.doc_id = d.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		result SearchResult
		vec    Embedding
	}
	var candidates []scored
	for rows.Next() {
		var blob []byte
		var sr SearchResult
		err := rows.Scan(&blob,
			&sr.Chunk.ID, &sr.Chunk.DocID, &sr.Chunk.Index,
			&sr.Chunk.Content, &sr.Chunk.Heading, &sr.Chunk.CreatedAt,
			&sr.Document.ID, &sr.Document.URL, &sr.Document.FilePath,
			&sr.Document.Title, &sr.Document.Project, &sr.Document.Size,
			&sr.Document.CreatedAt, &sr.Document.UpdatedAt)
		if err != nil {
			return nil, err
		}
		vec := blobToEmbedding(blob)
		sr.Score = cosineSimilarity(target, vec)
		candidates = append(candidates, scored{result: sr, vec: vec})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort by score descending
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].result.Score > candidates[i].result.Score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	if limit > len(candidates) {
		limit = len(candidates)
	}
	results := make([]SearchResult, limit)
	for i := 0; i < limit; i++ {
		results[i] = candidates[i].result
	}
	return results, nil
}

// UpdateDocumentChecksum updates the checksum for change detection.
func (s *Store) UpdateDocumentChecksum(id, checksum string) error {
	_, err := s.db.Exec("UPDATE documents SET checksum=?, updated_at=CURRENT_TIMESTAMP WHERE id=?", checksum, id)
	return err
}

// ----- helpers -----

func embeddingToBlob(e Embedding) []byte {
	b := make([]byte, len(e)*4)
	for i, v := range e {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(v))
	}
	return b
}

func blobToEmbedding(b []byte) Embedding {
	var e Embedding
	n := len(b) / 4
	if n > len(e) {
		n = len(e)
	}
	for i := 0; i < n; i++ {
		e[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return e
}

func cosineSimilarity(a, b Embedding) float64 {
	var dot, na, nb float64
	for i := 0; i < 384; i++ {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func scanResults(rows *sql.Rows) ([]SearchResult, error) {
	var results []SearchResult
	for rows.Next() {
		var sr SearchResult
		err := rows.Scan(
			&sr.Chunk.ID, &sr.Chunk.DocID, &sr.Chunk.Index,
			&sr.Chunk.Content, &sr.Chunk.Heading, &sr.Chunk.CreatedAt,
			&sr.Document.ID, &sr.Document.URL, &sr.Document.FilePath,
			&sr.Document.Title, &sr.Document.Project, &sr.Document.Size,
			&sr.Document.CreatedAt, &sr.Document.UpdatedAt,
			&sr.Score)
		if err != nil {
			return nil, err
		}
		results = append(results, sr)
	}
	return results, rows.Err()
}

func isFTS5Duplicate(err error) bool {
	if e, ok := err.(*sqlite.Error); ok {
		return e.Code() == 1555 // SQLITE_CONSTRAINT_PRIMARYKEY
	}
	return false
}

// InitTimestamp helpers
func init() {
	// Ensure timezone loaded
	_ = time.Local
}

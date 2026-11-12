package knowledge

import "fmt"

// shortIDLen is the human-facing prefix length used by list/show output.
const shortIDLen = 12

// ShortID returns the display prefix for a document ID. Short IDs (for example
// hand-written test fixtures) are returned unchanged instead of panicking.
func ShortID(id string) string {
	if len(id) <= shortIDLen {
		return id
	}
	return id[:shortIDLen]
}

// FindByIDPrefix resolves a unique document by ID prefix. An empty prefix
// matches nothing; two or more matches return an ambiguity error.
func (s *Store) FindByIDPrefix(prefix string) (*Document, error) {
	if prefix == "" {
		return nil, nil
	}
	rows, err := s.db.Query("SELECT id FROM documents WHERE id LIKE ? || '%'", prefix)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var fullID string
	if rows.Next() {
		if err := rows.Scan(&fullID); err != nil {
			return nil, err
		}
	}
	if rows.Next() {
		return nil, fmt.Errorf("ambiguous prefix: multiple documents match %q", prefix)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if fullID == "" {
		return nil, nil
	}
	return s.GetDocument(fullID)
}

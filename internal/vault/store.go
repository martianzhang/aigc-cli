package vault

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// VaultDoc represents a document in the vault.
type VaultDoc struct {
	ID        string    `json:"id"` // SHA256 of plaintext
	URL       string    `json:"url,omitempty"`
	FilePath  string    `json:"filepath,omitempty"`
	Title     string    `json:"title,omitempty"`
	Size      int64     `json:"size"` // plaintext size
	CreatedAt time.Time `json:"created_at"`
}

// Vault manages encrypted document storage.
type Vault struct {
	baseDir string // ~/.config/aigc-cli/vault
}

// Open opens or initializes the vault directory.
func Open(baseDir string) (*Vault, error) {
	if err := os.MkdirAll(filepath.Join(baseDir, "docs"), 0755); err != nil {
		return nil, fmt.Errorf("create vault dir: %w", err)
	}
	return &Vault{baseDir: baseDir}, nil
}

// BaseDir returns the vault directory path.
func (v *Vault) BaseDir() string { return v.baseDir }

// docPath returns the path to an encrypted doc file.
func (v *Vault) docPath(docID string) string {
	return filepath.Join(v.baseDir, "docs", docID+".age")
}

// metadataPath returns the path to the metadata file.
func (v *Vault) metadataPath() string {
	return filepath.Join(v.baseDir, "metadata.json")
}

// Save stores an encrypted document.
func (v *Vault) Save(doc *VaultDoc, plaintext []byte) error {
	// Encrypt
	ciphertext, err := Encrypt(plaintext)
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	// Write encrypted file
	if err := os.WriteFile(v.docPath(doc.ID), ciphertext, 0644); err != nil {
		return fmt.Errorf("write doc: %w", err)
	}

	// Update metadata
	return v.addMetadata(doc)
}

// Load retrieves and decrypts a document.
func (v *Vault) Load(docID string) (*VaultDoc, []byte, error) {
	ciphertext, err := os.ReadFile(v.docPath(docID))
	if err != nil {
		return nil, nil, fmt.Errorf("read encrypted doc: %w", err)
	}

	plaintext, err := Decrypt(ciphertext)
	if err != nil {
		return nil, nil, fmt.Errorf("decrypt: %w", err)
	}

	doc, err := v.getMetadata(docID)
	if err != nil {
		doc = &VaultDoc{ID: docID, Title: docID[:12], Size: int64(len(plaintext))}
	}

	return doc, plaintext, nil
}

// Delete removes a document from the vault.
func (v *Vault) Delete(docID string) error {
	if err := os.Remove(v.docPath(docID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return v.removeMetadata(docID)
}

// List returns all vault document metadata (titles only, no content).
func (v *Vault) List() ([]VaultDoc, error) {
	return v.readMetadata()
}

// metadata helpers

func (v *Vault) addMetadata(doc *VaultDoc) error {
	docs, err := v.readMetadata()
	if err != nil {
		docs = nil
	}

	// Replace if exists, else append
	found := false
	for i, d := range docs {
		if d.ID == doc.ID {
			docs[i] = *doc
			found = true
			break
		}
	}
	if !found {
		docs = append(docs, *doc)
	}

	return v.writeMetadata(docs)
}

func (v *Vault) getMetadata(docID string) (*VaultDoc, error) {
	docs, err := v.readMetadata()
	if err != nil {
		return nil, err
	}
	for _, d := range docs {
		if d.ID == docID {
			return &d, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (v *Vault) removeMetadata(docID string) error {
	docs, err := v.readMetadata()
	if err != nil {
		return nil
	}
	var kept []VaultDoc
	for _, d := range docs {
		if d.ID != docID {
			kept = append(kept, d)
		}
	}
	return v.writeMetadata(kept)
}

func (v *Vault) readMetadata() ([]VaultDoc, error) {
	data, err := os.ReadFile(v.metadataPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var docs []VaultDoc
	if err := json.Unmarshal(data, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func (v *Vault) writeMetadata(docs []VaultDoc) error {
	data, err := json.MarshalIndent(docs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(v.metadataPath(), data, 0644)
}

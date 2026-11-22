package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Store is the on-disk ledger: one JSON file plus a photos/ directory.
type Store struct {
	Path string // full path of the JSON data file
}

type fileFormat struct {
	Version int    `json:"version"`
	Items   []Item `json:"items"`
}

// DataDir returns Hoshobako's data directory for this user, honouring
// HOSHOBAKO_DATA_DIR, then the platform convention
// (%APPDATA%\Hoshobako on Windows, ~/Library/Application Support/Hoshobako on
// macOS, $XDG_DATA_HOME/hoshobako or ~/.local/share/hoshobako elsewhere).
func DataDir() (string, error) {
	if d := os.Getenv("HOSHOBAKO_DATA_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Hoshobako"), nil
}

// OpenStore resolves the store location and ensures its directories exist.
func OpenStore() (*Store, error) {
	dir, err := DataDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "photos"), 0o755); err != nil {
		return nil, err
	}
	return &Store{Path: filepath.Join(dir, "hoshobako.json")}, nil
}

// Dir returns the directory holding the data file.
func (s *Store) Dir() string { return filepath.Dir(s.Path) }

// PhotosDir returns the directory that holds receipt photos.
func (s *Store) PhotosDir() string { return filepath.Join(s.Dir(), "photos") }

// Load reads all items. A missing file is an empty ledger, not an error.
func (s *Store) Load() ([]Item, error) {
	raw, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return []Item{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return []Item{}, nil
	}
	var f fileFormat
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("data file is corrupted: %w", err)
	}
	if f.Items == nil {
		return []Item{}, nil
	}
	return f.Items, nil
}

// Save writes all items atomically (temp file + rename), so a crash or a
// power cut mid-write can never truncate the ledger.
func (s *Store) Save(items []Item) error {
	f := fileFormat{Version: 1, Items: items}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

// Add validates and appends an item, assigning ID and timestamps.
func (s *Store) Add(it Item) (Item, error) {
	if err := it.Validate(); err != nil {
		return Item{}, err
	}
	items, err := s.Load()
	if err != nil {
		return Item{}, err
	}
	now := time.Now().Format(time.RFC3339)
	it.ID = NewID()
	it.CreatedAt = now
	it.UpdatedAt = now
	items = append(items, it)
	if err := s.Save(items); err != nil {
		return Item{}, err
	}
	return it, nil
}

// Update replaces the item with the same ID.
func (s *Store) Update(it Item) (Item, error) {
	if err := it.Validate(); err != nil {
		return Item{}, err
	}
	items, err := s.Load()
	if err != nil {
		return Item{}, err
	}
	for i := range items {
		if items[i].ID == it.ID {
			it.CreatedAt = items[i].CreatedAt
			it.UpdatedAt = time.Now().Format(time.RFC3339)
			items[i] = it
			return it, s.Save(items)
		}
	}
	return Item{}, fmt.Errorf("no such item: %s", it.ID)
}

// Delete removes the item with the given ID and its photo, if any.
func (s *Store) Delete(id string) error {
	items, err := s.Load()
	if err != nil {
		return err
	}
	out := items[:0]
	found := false
	var photo string
	for _, it := range items {
		if it.ID == id {
			found = true
			photo = it.Photo
			continue
		}
		out = append(out, it)
	}
	if !found {
		return fmt.Errorf("no such item: %s", id)
	}
	if err := s.Save(out); err != nil {
		return err
	}
	if photo != "" {
		_ = os.Remove(filepath.Join(s.PhotosDir(), filepath.Base(photo)))
	}
	return nil
}

// Get returns the item with the given ID.
func (s *Store) Get(id string) (Item, bool, error) {
	items, err := s.Load()
	if err != nil {
		return Item{}, false, err
	}
	for _, it := range items {
		if it.ID == id {
			return it, true, nil
		}
	}
	return Item{}, false, nil
}

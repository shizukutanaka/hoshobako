// Package ledger implements the local-first warranty & expiry ledger.
//
// All data lives in a single JSON file in the user's own data directory.
// Nothing is ever sent anywhere: Hoshobako has no network client code at all.
package ledger

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Category of a tracked item.
type Category string

const (
	CategoryWarranty Category = "warranty" // product warranty
	CategoryDocument Category = "document" // passport, licence, certificates
	CategoryFood     Category = "food"     // food / medicine / cosmetics expiry
	CategoryContract Category = "contract" // cancel-by dates, renewals
	CategoryOther    Category = "other"
)

// Item is one tracked thing with an expiry date.
type Item struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Category  Category `json:"category"`
	Store     string   `json:"store,omitempty"`     // where it was bought / issued
	Model     string   `json:"model,omitempty"`     // model or serial number
	Price     string   `json:"price,omitempty"`     // free-form, e.g. "¥12,800"
	Purchased string   `json:"purchased,omitempty"` // YYYY-MM-DD
	Months    int      `json:"months,omitempty"`    // warranty length in months (with Purchased)
	Expires   string   `json:"expires,omitempty"`   // YYYY-MM-DD, explicit (wins over Months)
	Photo     string   `json:"photo,omitempty"`     // file name inside the data dir's photos/
	Notes     string   `json:"notes,omitempty"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

const dateLayout = "2006-01-02"

// ParseDate parses a YYYY-MM-DD date.
func ParseDate(s string) (time.Time, error) {
	return time.ParseInLocation(dateLayout, s, time.Local)
}

// ExpiryDate returns the effective expiry date of the item.
// An explicit Expires wins; otherwise Purchased + Months is used.
func (it *Item) ExpiryDate() (time.Time, bool) {
	if it.Expires != "" {
		if t, err := ParseDate(it.Expires); err == nil {
			return t, true
		}
		return time.Time{}, false
	}
	if it.Purchased != "" && it.Months > 0 {
		if t, err := ParseDate(it.Purchased); err == nil {
			return t.AddDate(0, it.Months, 0), true
		}
	}
	return time.Time{}, false
}

// Status of an item relative to a reference day.
type Status string

const (
	StatusOK       Status = "ok"
	StatusExpiring Status = "expiring" // within the warning window
	StatusExpired  Status = "expired"
	StatusNoDate   Status = "nodate"
)

// DaysLeft returns whole days from now's date until expiry (negative when past).
func DaysLeft(now time.Time, expiry time.Time) int {
	n := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	e := time.Date(expiry.Year(), expiry.Month(), expiry.Day(), 0, 0, 0, 0, time.Local)
	return int(e.Sub(n).Hours() / 24)
}

// StatusAt classifies the item at time now. warnDays is the warning window.
func (it *Item) StatusAt(now time.Time, warnDays int) (Status, int, bool) {
	exp, ok := it.ExpiryDate()
	if !ok {
		return StatusNoDate, 0, false
	}
	d := DaysLeft(now, exp)
	switch {
	case d < 0:
		return StatusExpired, d, true
	case d <= warnDays:
		return StatusExpiring, d, true
	default:
		return StatusOK, d, true
	}
}

// Validate checks the item's fields. It returns a plain-language error.
func (it *Item) Validate() error {
	if it.Name == "" {
		return errors.New("name is required")
	}
	switch it.Category {
	case CategoryWarranty, CategoryDocument, CategoryFood, CategoryContract, CategoryOther:
	default:
		return fmt.Errorf("unknown category: %q", it.Category)
	}
	if it.Purchased != "" {
		if _, err := ParseDate(it.Purchased); err != nil {
			return fmt.Errorf("purchased must be YYYY-MM-DD: %q", it.Purchased)
		}
	}
	if it.Expires != "" {
		if _, err := ParseDate(it.Expires); err != nil {
			return fmt.Errorf("expires must be YYYY-MM-DD: %q", it.Expires)
		}
	}
	if it.Months < 0 {
		return errors.New("months must not be negative")
	}
	if it.Expires == "" && it.Months == 0 {
		return errors.New("set either an expiry date or a purchase date with a warranty length")
	}
	if it.Expires == "" && it.Months > 0 && it.Purchased == "" {
		return errors.New("a warranty length needs a purchase date")
	}
	return nil
}

// NewID returns a random 12-hex-character identifier.
func NewID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is not expected; fall back to time-based bytes.
		n := time.Now().UnixNano()
		for i := range b {
			b[i] = byte(n >> (8 * i))
		}
	}
	return hex.EncodeToString(b[:])
}

// SortByExpiry returns a copy of items sorted by effective expiry date
// (items without a date go last), then by name.
func SortByExpiry(items []Item) []Item {
	out := make([]Item, len(items))
	copy(out, items)
	sort.SliceStable(out, func(i, j int) bool {
		ei, oki := out[i].ExpiryDate()
		ej, okj := out[j].ExpiryDate()
		if oki != okj {
			return oki
		}
		if oki && !ei.Equal(ej) {
			return ei.Before(ej)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Due returns the items expired or expiring within warnDays, soonest first.
func Due(items []Item, now time.Time, warnDays int) []Item {
	var out []Item
	for _, it := range items {
		if st, _, ok := it.StatusAt(now, warnDays); ok && st != StatusOK {
			out = append(out, it)
		}
	}
	return SortByExpiry(out)
}

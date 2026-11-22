package ledger

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ExportJSON serialises items in Hoshobako's open interchange format
// (the same shape as the data file, so an export is also a backup).
func ExportJSON(items []Item) ([]byte, error) {
	return json.MarshalIndent(fileFormat{Version: 1, Items: items}, "", "  ")
}

// ImportJSON parses an exported (or backed-up) ledger file.
func ImportJSON(raw []byte) ([]Item, error) {
	var f fileFormat
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("not a Hoshobako JSON file: %w", err)
	}
	for i := range f.Items {
		if err := f.Items[i].Validate(); err != nil {
			return nil, fmt.Errorf("item %d (%q): %w", i, f.Items[i].Name, err)
		}
		if f.Items[i].ID == "" {
			f.Items[i].ID = NewID()
		}
	}
	if f.Items == nil {
		return []Item{}, nil
	}
	return f.Items, nil
}

// ExportCSV renders the ledger as CSV with a stable column set.
func ExportCSV(items []Item) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	header := []string{"name", "category", "store", "model", "price", "purchased", "warranty_months", "expires", "notes"}
	if err := w.Write(header); err != nil {
		return nil, err
	}
	for _, it := range SortByExpiry(items) {
		exp := ""
		if t, ok := it.ExpiryDate(); ok {
			exp = t.Format(dateLayout)
		}
		months := ""
		if it.Months > 0 {
			months = fmt.Sprint(it.Months)
		}
		rec := []string{it.Name, string(it.Category), it.Store, it.Model, it.Price, it.Purchased, months, exp, it.Notes}
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func icsEscape(s string) string {
	r := strings.NewReplacer("\\", "\\\\", ";", "\\;", ",", "\\,", "\n", "\\n")
	return r.Replace(s)
}

// ExportICS renders one all-day calendar event per dated item, on its
// expiry date, so any calendar app can remind the user. This is how a
// fully offline tool does reminders: it hands the dates to the calendar
// the user already has, instead of running a server.
func ExportICS(items []Item, now time.Time) []byte {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Hoshobako//Expiry Ledger//EN\r\n")
	stamp := now.UTC().Format("20060102T150405Z")
	for _, it := range items {
		exp, ok := it.ExpiryDate()
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "BEGIN:VEVENT\r\nUID:hoshobako-%s@local\r\nDTSTAMP:%s\r\nDTSTART;VALUE=DATE:%s\r\nSUMMARY:%s\r\n",
			it.ID, stamp, exp.Format("20060102"), icsEscape("Expires: "+it.Name))
		desc := fmt.Sprintf("Category: %s", it.Category)
		if it.Store != "" {
			desc += "\nStore: " + it.Store
		}
		if it.Notes != "" {
			desc += "\n" + it.Notes
		}
		fmt.Fprintf(&b, "DESCRIPTION:%s\r\n", icsEscape(desc))
		b.WriteString("BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-P30D\r\nEND:VALARM\r\n")
		b.WriteString("BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-P7D\r\nEND:VALARM\r\n")
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	return []byte(b.String())
}

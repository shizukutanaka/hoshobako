package ledger

import (
	"os"
	"strings"
	"testing"
	"time"
)

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := ParseDate(s)
	if err != nil {
		t.Fatalf("ParseDate(%q): %v", s, err)
	}
	return d
}

func TestExpiryFromPurchasePlusMonths(t *testing.T) {
	it := Item{Name: "Fridge", Category: CategoryWarranty, Purchased: "2025-03-15", Months: 12}
	exp, ok := it.ExpiryDate()
	if !ok {
		t.Fatal("expected an expiry date")
	}
	if got := exp.Format(dateLayout); got != "2026-03-15" {
		t.Fatalf("expiry = %s, want 2026-03-15", got)
	}
}

func TestExplicitExpiryWins(t *testing.T) {
	it := Item{Name: "Passport", Category: CategoryDocument, Purchased: "2020-01-01", Months: 120, Expires: "2030-06-30"}
	exp, _ := it.ExpiryDate()
	if got := exp.Format(dateLayout); got != "2030-06-30" {
		t.Fatalf("expiry = %s, want 2030-06-30", got)
	}
}

func TestStatusBoundaries(t *testing.T) {
	now := mustDate(t, "2026-11-22")
	cases := []struct {
		expires string
		want    Status
	}{
		{"2026-11-21", StatusExpired},
		{"2026-11-22", StatusExpiring}, // today counts as expiring, not expired
		{"2026-12-22", StatusExpiring}, // exactly 30 days
		{"2026-12-23", StatusOK},
	}
	for _, c := range cases {
		it := Item{Name: "x", Category: CategoryOther, Expires: c.expires}
		st, _, ok := it.StatusAt(now, 30)
		if !ok || st != c.want {
			t.Errorf("expires %s: status = %s, want %s", c.expires, st, c.want)
		}
	}
}

func TestValidate(t *testing.T) {
	bad := []Item{
		{Category: CategoryWarranty, Expires: "2030-01-01"},                          // no name
		{Name: "x", Category: "bogus", Expires: "2030-01-01"},                        // bad category
		{Name: "x", Category: CategoryOther, Expires: "30/01/2030"},                  // bad date
		{Name: "x", Category: CategoryOther},                                         // no date at all
		{Name: "x", Category: CategoryWarranty, Months: 12},                          // months without purchase
		{Name: "x", Category: CategoryWarranty, Purchased: "2025-01-01", Months: -1}, // negative months
	}
	for i, it := range bad {
		if err := it.Validate(); err == nil {
			t.Errorf("case %d: expected a validation error, got nil", i)
		}
	}
	good := Item{Name: "x", Category: CategoryWarranty, Purchased: "2025-01-01", Months: 24}
	if err := good.Validate(); err != nil {
		t.Errorf("valid item rejected: %v", err)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: dir + "/hoshobako.json"}
	if err := os.MkdirAll(s.PhotosDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	added, err := s.Add(Item{Name: "Washer", Category: CategoryWarranty, Purchased: "2026-01-10", Months: 24, Store: "Denkiya"})
	if err != nil {
		t.Fatal(err)
	}
	if added.ID == "" {
		t.Fatal("Add did not assign an ID")
	}
	items, err := s.Load()
	if err != nil || len(items) != 1 {
		t.Fatalf("Load = %v items, err %v", len(items), err)
	}
	added.Name = "Washer DX"
	if _, err := s.Update(added); err != nil {
		t.Fatal(err)
	}
	got, ok, _ := s.Get(added.ID)
	if !ok || got.Name != "Washer DX" {
		t.Fatalf("Get after Update = %+v", got)
	}
	if err := s.Delete(added.ID); err != nil {
		t.Fatal(err)
	}
	items, _ = s.Load()
	if len(items) != 0 {
		t.Fatalf("after Delete: %d items remain", len(items))
	}
}

func TestImportExportRoundTrip(t *testing.T) {
	items := []Item{
		{ID: "abc123", Name: "A", Category: CategoryFood, Expires: "2026-12-01"},
		{ID: "def456", Name: "B", Category: CategoryWarranty, Purchased: "2025-05-05", Months: 36},
	}
	raw, err := ExportJSON(items)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ImportJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != 2 || back[0].Name != "A" || back[1].Name != "B" {
		t.Fatalf("round trip mismatch: %+v", back)
	}
	if _, err := ImportJSON([]byte("{not json")); err == nil {
		t.Fatal("corrupt JSON accepted")
	}
}

func TestCSVAndICS(t *testing.T) {
	items := []Item{{ID: "x1", Name: "Milk, fresh", Category: CategoryFood, Expires: "2026-11-25"}}
	csvRaw, err := ExportCSV(items)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(csvRaw), "\"Milk, fresh\"") {
		t.Fatalf("CSV quoting broken: %s", csvRaw)
	}
	ics := string(ExportICS(items, mustDate(t, "2026-11-22")))
	for _, want := range []string{"BEGIN:VCALENDAR", "DTSTART;VALUE=DATE:20261125", "Expires: Milk\\, fresh", "TRIGGER:-P30D", "END:VCALENDAR"} {
		if !strings.Contains(ics, want) {
			t.Errorf("ICS missing %q", want)
		}
	}
}

func TestDueOrdering(t *testing.T) {
	now := mustDate(t, "2026-11-22")
	items := []Item{
		{Name: "later", Category: CategoryOther, Expires: "2027-06-01"},
		{Name: "soon", Category: CategoryOther, Expires: "2026-11-30"},
		{Name: "gone", Category: CategoryOther, Expires: "2026-11-01"},
		{Name: "nodate", Category: CategoryOther, Purchased: "2026-01-01"}, // no months, no expires: invalid in practice; ExpiryDate false
	}
	due := Due(items, now, 30)
	if len(due) != 2 {
		t.Fatalf("Due returned %d items, want 2", len(due))
	}
	if due[0].Name != "gone" || due[1].Name != "soon" {
		t.Fatalf("Due order = %s, %s; want gone, soon", due[0].Name, due[1].Name)
	}
}

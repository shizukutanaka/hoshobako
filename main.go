// Hoshobako — a local-first warranty & expiry ledger.
//
// One binary. It serves a small web UI on 127.0.0.1 only, keeps every
// record in a single JSON file in the user's own data folder, and has no
// account, no cloud and no network client code. Reminders are handed to
// the calendar app the user already has, via an .ics export.
package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"hoshobako/internal/ledger"
)

//go:embed web/index.html
var indexHTML []byte

const (
	version  = "0.1.0"
	warnDays = 30
	marker   = "HOSHOBAKO-APP"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve":
			exit(serve())
			return
		case "add":
			exit(cliAdd(os.Args[2:]))
			return
		case "list":
			exit(cliList())
			return
		case "due":
			exit(cliDue())
			return
		case "export":
			exit(cliExport(os.Args[2:]))
			return
		case "import":
			exit(cliImport(os.Args[2:]))
			return
		case "version", "-version", "--version":
			fmt.Println("hoshobako", version)
			return
		default:
			fmt.Fprintf(os.Stderr, "hoshobako: unknown command %q\n", os.Args[1])
			printUsage()
			os.Exit(1)
		}
	}
	exit(serve())
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: hoshobako [serve|add|list|due|export|import|version]")
	fmt.Fprintln(os.Stderr, "  serve            start the web UI (default when no command is given)")
	fmt.Fprintln(os.Stderr, "  add ...          add an item (see: hoshobako add -h)")
	fmt.Fprintln(os.Stderr, "  list             list all items")
	fmt.Fprintln(os.Stderr, "  due              list items expired or expiring soon")
	fmt.Fprintln(os.Stderr, "  export json|csv|ics [file]   write the ledger out")
	fmt.Fprintln(os.Stderr, "  import <file.json>           restore the ledger from a JSON export")
	fmt.Fprintln(os.Stderr, "  version          print the version")
}

func exit(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "hoshobako:", err)
		os.Exit(1)
	}
}

// ---------- server ----------

type server struct {
	store *ledger.Store
	mu    sync.Mutex
}

type itemView struct {
	ledger.Item
	Status    ledger.Status `json:"status"`
	DaysLeft  int           `json:"days_left"`
	HasDate   bool          `json:"has_date"`
	ExpiresOn string        `json:"expires_on,omitempty"`
}

func view(it ledger.Item, now time.Time) itemView {
	st, d, ok := it.StatusAt(now, warnDays)
	v := itemView{Item: it, Status: st, DaysLeft: d, HasDate: ok}
	if exp, has := it.ExpiryDate(); has {
		v.ExpiresOn = exp.Format("2006-01-02")
	}
	return v
}

func (s *server) loadViews() ([]itemView, error) {
	items, err := s.store.Load()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]itemView, 0, len(items))
	for _, it := range ledger.SortByExpiry(items) {
		out = append(out, view(it, now))
	}
	return out, nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, err.Error())
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	mux.HandleFunc("GET /api/items", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		views, err := s.loadViews()
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, views)
	})
	mux.HandleFunc("POST /api/items", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var it ledger.Item
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&it); err != nil {
			writeErr(w, 400, err)
			return
		}
		it.ID = ""
		it.Photo = ""
		saved, err := s.store.Add(it)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, 201, view(saved, time.Now()))
	})
	mux.HandleFunc("PUT /api/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var it ledger.Item
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&it); err != nil {
			writeErr(w, 400, err)
			return
		}
		it.ID = r.PathValue("id")
		old, ok, err := s.store.Get(it.ID)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		if !ok {
			writeErr(w, 404, errors.New("no such item"))
			return
		}
		it.Photo = old.Photo // photos change only via the photo endpoint
		saved, err := s.store.Update(it)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, 200, view(saved, time.Now()))
	})
	mux.HandleFunc("DELETE /api/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.store.Delete(r.PathValue("id")); err != nil {
			writeErr(w, 404, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/items/{id}/photo", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		id := r.PathValue("id")
		it, ok, err := s.store.Get(id)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		if !ok {
			writeErr(w, 404, errors.New("no such item"))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 20<<20)
		f, hdr, err := r.FormFile("photo")
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		defer f.Close()
		ext := strings.ToLower(filepath.Ext(hdr.Filename))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		default:
			writeErr(w, 400, errors.New("photo must be a jpg, png, gif or webp image"))
			return
		}
		name := id + ext
		dst, err := os.Create(filepath.Join(s.store.PhotosDir(), name))
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		if _, err := io.Copy(dst, f); err != nil {
			dst.Close()
			writeErr(w, 500, err)
			return
		}
		dst.Close()
		if it.Photo != "" && it.Photo != name {
			_ = os.Remove(filepath.Join(s.store.PhotosDir(), it.Photo))
		}
		it.Photo = name
		if _, err := s.store.Update(it); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]string{"photo": name})
	})
	mux.HandleFunc("GET /photos/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.PathValue("name"))
		http.ServeFile(w, r, filepath.Join(s.store.PhotosDir(), name))
	})
	export := func(name, ctype string, gen func(items []ledger.Item) ([]byte, error)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			s.mu.Lock()
			defer s.mu.Unlock()
			items, err := s.store.Load()
			if err != nil {
				writeErr(w, 500, err)
				return
			}
			raw, err := gen(items)
			if err != nil {
				writeErr(w, 500, err)
				return
			}
			w.Header().Set("Content-Type", ctype)
			w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
			_, _ = w.Write(raw)
		}
	}
	mux.HandleFunc("GET /api/export.json", export("hoshobako.json", "application/json", ledger.ExportJSON))
	mux.HandleFunc("GET /api/export.csv", export("hoshobako.csv", "text/csv; charset=utf-8", ledger.ExportCSV))
	mux.HandleFunc("GET /api/export.ics", export("hoshobako.ics", "text/calendar; charset=utf-8",
		func(items []ledger.Item) ([]byte, error) { return ledger.ExportICS(items, time.Now()), nil }))
	mux.HandleFunc("POST /api/import", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		items, err := ledger.ImportJSON(raw)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		if err := s.store.Save(items); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]int{"imported": len(items)})
	})
	return mux
}

func isOurServer(port int) bool {
	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return strings.Contains(string(body), marker)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func serve() error {
	store, err := ledger.OpenStore()
	if err != nil {
		return err
	}
	s := &server{store: store}
	for port := 8771; port <= 8780; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			if isOurServer(port) {
				url := fmt.Sprintf("http://127.0.0.1:%d", port)
				fmt.Println("Hoshobako is already running at", url)
				openBrowser(url)
				return nil
			}
			continue // some other app owns this port; try the next one
		}
		url := fmt.Sprintf("http://127.0.0.1:%d", port)
		fmt.Println("Hoshobako", version, "—", url)
		fmt.Println("Data file:", store.Path)
		fmt.Println("Press Ctrl+C to stop.")
		openBrowser(url)
		return http.Serve(ln, s.routes())
	}
	return errors.New("no free port in 8771-8780")
}

// ---------- CLI ----------

func cliAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	name := fs.String("name", "", "item name (required)")
	category := fs.String("category", "warranty", "warranty|document|food|contract|other")
	store := fs.String("store", "", "where it was bought / issued")
	model := fs.String("model", "", "model or serial number")
	price := fs.String("price", "", "price paid, free-form")
	purchased := fs.String("purchased", "", "purchase date YYYY-MM-DD")
	months := fs.Int("months", 0, "warranty length in months")
	expires := fs.String("expires", "", "expiry date YYYY-MM-DD")
	notes := fs.String("notes", "", "notes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	st, err := ledger.OpenStore()
	if err != nil {
		return err
	}
	saved, err := st.Add(ledger.Item{
		Name: *name, Category: ledger.Category(*category), Store: *store,
		Model: *model, Price: *price, Purchased: *purchased,
		Months: *months, Expires: *expires, Notes: *notes,
	})
	if err != nil {
		return err
	}
	fmt.Println("added:", saved.ID, saved.Name)
	return nil
}

func printItems(items []ledger.Item) {
	now := time.Now()
	for _, it := range ledger.SortByExpiry(items) {
		st, d, ok := it.StatusAt(now, warnDays)
		when := "no date"
		if ok {
			exp, _ := it.ExpiryDate()
			when = fmt.Sprintf("%s (%d days left, %s)", exp.Format("2006-01-02"), d, st)
		}
		fmt.Printf("%-40s %-10s %s\n", it.Name, it.Category, when)
	}
}

func cliList() error {
	st, err := ledger.OpenStore()
	if err != nil {
		return err
	}
	items, err := st.Load()
	if err != nil {
		return err
	}
	printItems(items)
	return nil
}

func cliDue() error {
	st, err := ledger.OpenStore()
	if err != nil {
		return err
	}
	items, err := st.Load()
	if err != nil {
		return err
	}
	due := ledger.Due(items, time.Now(), warnDays)
	if len(due) == 0 {
		fmt.Println("nothing expired or expiring within", warnDays, "days")
		return nil
	}
	printItems(due)
	return nil
}

func cliExport(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: hoshobako export json|csv|ics [file]")
	}
	st, err := ledger.OpenStore()
	if err != nil {
		return err
	}
	items, err := st.Load()
	if err != nil {
		return err
	}
	var raw []byte
	switch args[0] {
	case "json":
		raw, err = ledger.ExportJSON(items)
	case "csv":
		raw, err = ledger.ExportCSV(items)
	case "ics":
		raw = ledger.ExportICS(items, time.Now())
	default:
		return fmt.Errorf("unknown format: %s", args[0])
	}
	if err != nil {
		return err
	}
	if len(args) > 1 {
		return os.WriteFile(args[1], raw, 0o644)
	}
	_, err = os.Stdout.Write(raw)
	return err
}

// cliImport restores the ledger from a JSON export (the counterpart of
// "export json"). It validates the whole file before touching the
// current ledger, and when the current ledger is not empty it first
// writes a timestamped backup of it into the data directory, so an
// import never silently destroys existing records.
func cliImport(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: hoshobako import <file.json>")
	}
	raw, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	items, err := ledger.ImportJSON(raw)
	if err != nil {
		return fmt.Errorf("invalid backup file: %w", err)
	}
	st, err := ledger.OpenStore()
	if err != nil {
		return err
	}
	current, err := st.Load()
	if err != nil {
		return err
	}
	backupPath := ""
	if len(current) > 0 {
		backup, err := ledger.ExportJSON(current)
		if err != nil {
			return err
		}
		backupPath = filepath.Join(st.Dir(), "backup-before-import-"+time.Now().Format("20060102-150405")+".json")
		if err := os.WriteFile(backupPath, backup, 0o644); err != nil {
			return fmt.Errorf("could not back up the current ledger, import aborted: %w", err)
		}
	}
	if err := st.Save(items); err != nil {
		return err
	}
	if backupPath != "" {
		fmt.Printf("imported %d items (previous %d items backed up to %s)\n", len(items), len(current), backupPath)
	} else {
		fmt.Printf("imported %d items\n", len(items))
	}
	return nil
}

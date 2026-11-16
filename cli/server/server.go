// Package server serves Dora Studio locally with a read-only HTTP API.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/MehulxBuilds/dora/engine"
	frontend "github.com/MehulxBuilds/dora/studio"
)

// Source must return a detached inspection, never an open file-backed DB.
type Source func() (*engine.Inspection, error)

type Options struct {
	Port      int // Zero selects an available port.
	Name      string
	Databases map[string]Source // Optional directory-discovered databases, keyed by display name.
	Mode      string            // Display label, normally live or offline.
	NoBrowser bool
	// Ready receives the local URL once the listener is bound.
	Ready func(string)
}

// Start serves a live DB without taking ownership of it or closing it.
// Each API request captures a consistent view under the engine read lock.
func Start(ctx context.Context, db *engine.DB, options Options) error {
	options.Mode = "live"
	return Serve(ctx, db.Inspect, options)
}

// Serve binds only to IPv4 loopback and shuts down when ctx is cancelled.
func Serve(ctx context.Context, source Source, options Options) error {
	if options.Port < 0 || options.Port > 65535 {
		return fmt.Errorf("invalid port")
	}
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", options.Port))
	if err != nil {
		return err
	}
	defer listener.Close()
	address := "http://" + listener.Addr().String()
	server := &http.Server{Handler: NewHandler(source, options), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, WriteTimeout: 30 * time.Second}
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdown)
		case <-stopped:
		}
	}()
	if options.Ready != nil {
		options.Ready(address)
	}
	if !options.NoBrowser {
		if err := openBrowser(address); err != nil {
			fmt.Printf("Open %s in your browser (%v)\n", address, err)
		}
	}
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func openBrowser(address string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	case "darwin":
		command = exec.Command("open", address)
	default:
		command = exec.Command("xdg-open", address)
	}
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}

// NewHandler exposes GET/HEAD only. It rejects non-loopback Host headers and
// cross-origin requests; no CORS headers or mutation routes are provided.
func NewHandler(source Source, options Options) http.Handler {
	assets, err := fs.Sub(frontend.Assets, "dist")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("Cache-Control", "no-store")
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			http.Error(w, "local requests only", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil || parsed.Host != r.Host || parsed.Scheme != "http" {
				http.Error(w, "cross-origin requests disabled", http.StatusForbidden)
				return
			}
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "cross-site requests disabled", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Studio is read-only", http.StatusMethodNotAllowed)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			files.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/api/databases" {
			names := make([]string, 0, len(options.Databases))
			for name := range options.Databases {
				names = append(names, name)
			}
			if len(names) == 0 {
				names = append(names, options.Name)
			}
			sort.Strings(names)
			writeJSON(w, 200, names)
			return
		}
		if r.URL.Path != "/api/overview" && r.URL.Path != "/api/record" && r.URL.Path != "/api/lookup" {
			http.NotFound(w, r)
			return
		}
		selectedSource := source
		selectedName := options.Name
		if name := r.URL.Query().Get("db"); name != "" && name != options.Name {
			var ok bool
			selectedSource, ok = options.Databases[name]
			if !ok {
				writeJSON(w, 404, map[string]string{"error": "Database not found"})
				return
			}
			selectedName = name
		}
		view, err := selectedSource()
		if err != nil {
			writeJSON(w, 503, map[string]string{"error": err.Error()})
			return
		}
		switch r.URL.Path {
		case "/api/overview":
			type summary struct {
				Key     string `json:"key"`
				Type    string `json:"type"`
				Preview string `json:"preview"`
			}
			records := make([]summary, 0, len(view.Records))
			for key, value := range view.Records {
				data, err := value.MarshalJSON()
				if err != nil {
					writeJSON(w, 500, map[string]string{"error": err.Error()})
					return
				}
				preview := string(data)
				runes := []rune(preview)
				if len(runes) > 160 {
					preview = string(runes[:160]) + "..."
				}
				records = append(records, summary{key, value.Type().String(), preview})
			}
			sort.Slice(records, func(i, j int) bool { return records[i].Key < records[j].Key })
			writeJSON(w, 200, map[string]interface{}{"name": selectedName, "mode": options.Mode, "seq": fmt.Sprint(view.Seq), "hash": fmt.Sprintf("%x", view.Hash), "records": records, "indexes": view.Indexes})
		case "/api/record":
			value, ok := view.Records[r.URL.Query().Get("key")]
			if !ok {
				writeJSON(w, 404, map[string]string{"error": "Key not found"})
				return
			}
			refs := make([]string, 0)
			collectReferences(value, &refs)
			sort.Strings(refs)
			writeJSON(w, 200, map[string]interface{}{"value": value, "type": value.Type().String(), "references": refs})
		case "/api/lookup":
			var value engine.Value
			if err := json.Unmarshal([]byte(r.URL.Query().Get("value")), &value); err != nil {
				writeJSON(w, 400, map[string]string{"error": "Enter a valid JSON scalar"})
				return
			}
			keys, err := view.Lookup(r.URL.Query().Get("name"), value)
			if err != nil {
				writeJSON(w, 400, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, 200, keys)
		}
	})
}

func collectReferences(v engine.Value, refs *[]string) {
	if key, ok := v.Reference(); ok {
		for _, k := range *refs {
			if k == key {
				return
			}
		}
		*refs = append(*refs, key)
	}
	if array, ok := v.Array(); ok {
		for _, item := range array {
			collectReferences(item, refs)
		}
	}
	if object, ok := v.Object(); ok {
		for _, item := range object {
			collectReferences(item, refs)
		}
	}
}
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	encoded, err := json.Marshal(data)
	if err != nil {
		http.Error(w, "Value cannot be represented as JSON", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}

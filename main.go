// vpngate-client: a VPN Gate client daemon with an embedded web UI.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"vpngate-client/internal/api"
	"vpngate-client/internal/desktop"
	"vpngate-client/internal/engine"
	"vpngate-client/internal/setup"
	"vpngate-client/internal/store"
	"vpngate-client/internal/vpn"
)

//go:embed all:web/dist
var webDist embed.FS

// version is stamped at release time via -ldflags "-X main.version=…".
var version = "dev"

func main() {
	// The root-owned launcher copy (Linux) does nothing else, whatever flags.
	if vpn.IsLauncher() {
		os.Exit(vpn.RunLauncher(os.Args[1:]))
	}
	listen := flag.String("listen", "127.0.0.1:8787", "address for the web UI / API")
	dataDir := flag.String("data", defaultDataDir(), "directory for settings, stats and list cache")
	noUI := flag.Bool("no-ui", false, "don't open the UI on startup (headless / service use)")
	browser := flag.Bool("browser", false, "open the UI in the default browser instead of an app window (Windows)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	// Internal modes the app launches itself with.
	helper := flag.Bool("setup-helper", false, "internal: privileged Linux setup step (run via pkexec)")
	helperUser := flag.String("setup-user", "", "internal: user to grant OpenVPN access to")
	uiWindow := flag.String("ui-window", "", "internal: run the native UI window for this URL")
	flag.Parse()

	switch {
	case *showVersion:
		fmt.Println(version)
		return
	case *helper:
		os.Exit(setup.RunHelper(*helperUser))
	case *uiWindow != "":
		os.Exit(desktop.RunWindow(*uiWindow, *dataDir))
	}
	if desktop.EnsureElevated() {
		return // an elevated copy of the app has taken over
	}

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		desktop.Fatal("create data dir: %v", err)
	}
	closeLog := setupLogging(*dataDir)
	defer closeLog()
	platformInit(*dataDir)
	defer platformCleanup(*dataDir)

	uiURL := localURL(*listen)
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		// Most likely we're already running: surface that instance instead.
		if surfaceExisting(uiURL) {
			return
		}
		desktop.Fatal("Could not start the web UI on %s: %v", *listen, err)
	}

	st, err := store.Load(filepath.Join(*dataDir, "state.json"))
	if err != nil {
		desktop.Fatal("load state: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	quit := make(chan struct{})
	var quitOnce sync.Once
	requestQuit := func() { quitOnce.Do(func() { close(quit) }) }

	eng := engine.New(st, *dataDir)
	sm := setup.New(*dataDir, func(s setup.State) {
		eng.Publish(engine.Event{Type: "setup", Data: s})
	})
	shell := desktop.New(uiURL, *dataDir, *browser, requestQuit)

	sm.Start(ctx)
	eng.Start(ctx, sm.WaitReady)

	web, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		desktop.Fatal("embedded web ui: %v", err)
	}
	app := api.App{Version: version, DataDir: *dataDir, Quit: requestQuit, Show: shell.Show}
	srv := &http.Server{Handler: api.New(ctx, eng, st, sm, app, web), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != http.ErrServerClosed {
			desktop.Fatal("http server: %v", err)
		}
	}()
	log.Printf("VPN Gate Client %s running at %s (data: %s)", version, uiURL, *dataDir)

	if !*noUI {
		shell.Show()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
	case <-quit:
	}
	log.Println("shutting down…")
	shell.Close()
	eng.Disconnect() // tear down any running openvpn process
	cancel()
	shutdownCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
	defer done()
	_ = srv.Shutdown(shutdownCtx)
}

// localURL is the browser-facing URL for a listen address (wildcard
// listeners are reached via loopback).
func localURL(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "http://" + listen
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// surfaceExisting asks an already-running instance to show its UI.
func surfaceExisting(url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url + "/api/info")
	if err != nil {
		return false
	}
	var info struct {
		Version string `json:"version"`
	}
	err = json.NewDecoder(resp.Body).Decode(&info)
	resp.Body.Close()
	if err != nil || info.Version == "" {
		return false // something else owns the port
	}
	desktop.AllowFocus()
	resp, err = client.Post(url+"/api/show", "application/json", nil)
	if err != nil {
		return false
	}
	resp.Body.Close()
	log.Printf("already running (%s) — showing the existing window", url)
	return true
}

// setupLogging mirrors the log to a file in the data dir, which is the only
// place to find it on Windows where the app has no console.
func setupLogging(dir string) func() {
	path := filepath.Join(dir, "vpngate-client.log")
	if fi, err := os.Stat(path); err == nil && fi.Size() > 2<<20 {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return func() {}
	}
	// File first: on Windows stderr is invalid and MultiWriter stops at the
	// first failing writer.
	log.SetOutput(io.MultiWriter(f, os.Stderr))
	return func() { f.Close() }
}

// defaultDataDir keeps state under the invoking user's home even when the
// binary is run via sudo, so stats survive across privilege modes.
func defaultDataDir() string {
	if su := os.Getenv("SUDO_USER"); su != "" && os.Geteuid() == 0 {
		home := filepath.Join("/home", su)
		if _, err := os.Stat(home); err == nil {
			return filepath.Join(home, ".config", "vpngate-client")
		}
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "./data"
	}
	return filepath.Join(dir, "vpngate-client")
}

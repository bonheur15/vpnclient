// vpngate-client: a VPN Gate client daemon with an embedded web UI.
package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"vpngate-client/internal/api"
	"vpngate-client/internal/engine"
	"vpngate-client/internal/store"
)

//go:embed all:web/dist
var webDist embed.FS

func main() {
	listen := flag.String("listen", "127.0.0.1:8787", "address for the web UI / API")
	dataDir := flag.String("data", defaultDataDir(), "directory for settings, stats and list cache")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}
	st, err := store.Load(filepath.Join(*dataDir, "state.json"))
	if err != nil {
		log.Fatalf("load state: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	eng := engine.New(st, *dataDir)
	eng.Start(ctx)

	web, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Fatalf("embedded web ui: %v", err)
	}

	srv := &http.Server{Addr: *listen, Handler: api.New(eng, st, web)}
	go func() {
		log.Printf("vpngate-client listening on http://%s (data: %s)", *listen, *dataDir)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Println("shutting down…")
	eng.Disconnect() // tear down any running openvpn process
	cancel()
	_ = srv.Shutdown(context.Background())
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

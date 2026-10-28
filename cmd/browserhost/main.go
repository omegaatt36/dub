//go:build browser

// browserhost serves the real frontend and application services for opt-in tests.
// It deliberately does not initialise Wails or emulate its native runtime.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/omegaatt36/dub/app"
	"github.com/omegaatt36/dub/internal/adapter/fs"
	"github.com/omegaatt36/dub/internal/adapter/regex"
	"github.com/omegaatt36/dub/internal/service"
)

func main() {
	root := flag.String("root", "", "temporary fixture directory (required)")
	web := flag.String("web", "web", "frontend asset directory")
	addr := flag.String("addr", "127.0.0.1:0", "numeric loopback listen address")
	flag.Parse()
	if *root == "" {
		log.Fatal("-root is required")
	}
	listener, err := listenLoopback(*addr)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	fileSystem := &fs.OSFileSystem{}
	dub := app.NewApp(fileSystem, service.NewScannerService(fileSystem),
		service.NewPatternService(&regex.Engine{}), service.NewRenamerService(fileSystem),
		app.WithHomeDir(*root), app.WithDirectoryPicker(func() (string, error) {
			return filepath.Join(*root, "first"), nil
		}))
	server := &http.Server{
		Handler:           browserHandler(dub.GetHandler(), *web, listener.Addr().String()),
		ReadHeaderTimeout: 5 * time.Second,
	}
	// Report the address only after binding. The test runner never has to
	// reserve a port and race another process to bind it again.
	fmt.Println("http://" + listener.Addr().String())
	log.Fatal(server.Serve(listener))
}

func listenLoopback(addr string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid listen address: %w", err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("browser host requires a numeric loopback address")
	}
	return net.Listen("tcp", addr)
}

func browserHandler(api http.Handler, web, address string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	// The native runtime is absent, not mocked. Browser tests must not use it.
	mux.HandleFunc("GET /wails/runtime.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("/", http.FileServer(http.Dir(web)))
	protected := http.NewCrossOriginProtection().Handler(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strict authority checking also rejects DNS-rebinding requests. The
		// fixture directory is not a sandbox, so the API must not be exposed
		// to unrelated browser origins even for a short-lived test process.
		if r.Host != address {
			http.Error(w, "unexpected browser host authority", http.StatusForbidden)
			return
		}
		protected.ServeHTTP(w, r)
	})
}

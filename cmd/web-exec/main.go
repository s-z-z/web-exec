// Package main is the entry point for the web-exec server.
// It wires together the config store, exec service, and config UI,
// then starts an HTTP server that serves both the execution endpoints
// and the configuration web interface.
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/s-z-z/web-exec/internal/config"
	"github.com/s-z-z/web-exec/internal/configui"
	"github.com/s-z-z/web-exec/internal/history"
	execsvc "github.com/s-z-z/web-exec/internal/exec"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:8080", "HTTP server listen address (use 0.0.0.0 for IPv4 only)")
	defaultDataDir := "data"
	if _, ok := os.LookupEnv("LOCALAPPDATA"); ok {
		defaultDataDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "web-exec")
	}
	dataDir := flag.String("data", defaultDataDir, "directory for persistent storage")
	timeout := flag.Duration("timeout", 30*time.Second, "script execution timeout")
	liveFlag := flag.String("live", "", "auto-exit after duration, e.g. 1d2h30m10s")
	token := flag.String("token", "", "bearer token for authentication (empty means no auth)")
	tlsFlag := flag.Bool("tls", false, "enable HTTPS with auto-generated self-signed certificate")
	flag.Parse()

	ctx := context.Background()

	// Initialize config store
	routesPath := filepath.Join(*dataDir, "routes.json")
	store := config.NewConfigStore(routesPath)
	if err := store.Load(ctx); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize history store
	historyPath := filepath.Join(*dataDir, "history.json")
	historyStore := history.NewHistoryStore(historyPath)
	if err := historyStore.Load(ctx); err != nil {
		log.Fatalf("Failed to load history: %v", err)
	}

	// Initialize exec service
	execService := execsvc.NewExecService(store)
	execService.SetTimeout(*timeout)

	// Initialize config UI handler
	configHandler := configui.NewHandler(store, historyStore)

	// Build HTTP mux
	mux := http.NewServeMux()

	// /exec/* — dynamic route execution
	execHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract route path from URL: /exec/deploy → /deploy
		routePath := strings.TrimPrefix(r.URL.Path, "/exec")
		if routePath == "" {
			http.Error(w, `{"error":"route path required"}`, http.StatusBadRequest)
			return
		}

		// Read request body for stdin
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		result, err := execService.HandleExec(r.Context(), routePath, body)
		if err != nil {
			if strings.Contains(err.Error(), "route not found") {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusNotFound)
			} else {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			}
			return
		}

		// Record execution history
		trigger := r.Header.Get("X-Trigger")
		if trigger != "test" {
			trigger = "exec"
		}
		if cfg, ok := store.GetByPath(routePath); ok {
			record := &history.ExecRecord{
				RouteID:    cfg.ID,
				RoutePath:  cfg.Path,
				Trigger:    trigger,
				Stdout:     result.Stdout,
				Stderr:     result.Stderr,
				ExitCode:   result.ExitCode,
				DurationMs: result.Duration,
				Error:      result.Error,
			}
			if err := historyStore.Add(r.Context(), record); err != nil {
				log.Printf("Failed to record history: %v", err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(result)
	})
	mux.Handle("/exec/", bearerAuth(*token, execHandler))

	// / — redirect to /config
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/config", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	// /config — configuration web UI (handler serves both UI and API)
	// Auth is applied only to API routes (paths containing "routes" or "history"),
	// so the HTML page loads without auth and shows the token input field.
	mux.Handle("/config", configAuth(*token, configHandler))
	mux.Handle("/config/", configAuth(*token, configHandler))

	// Build HTTP server with auth middleware
	srv := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second, // Longer for script execution
	}

	// Set up signal handler for graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down, saving config...")
		if err := store.Save(ctx); err != nil {
			log.Printf("Failed to save config on shutdown: %v", err)
		}
		if err := historyStore.Save(ctx); err != nil {
			log.Printf("Failed to save history on shutdown: %v", err)
		}
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Failed to shutdown server: %v", err)
		}
	}()

	// Handle -live flag
	if *liveFlag != "" {
		liveDuration, err := parseLiveDuration(*liveFlag)
		if err != nil {
			log.Fatalf("Invalid -live duration: %v", err)
		}
		log.Printf("Live duration set: %v", liveDuration)
		go func() {
			<-time.After(liveDuration)
			log.Println("Live duration expired, shutting down...")
			if err := store.Save(ctx); err != nil {
				log.Printf("Failed to save config on live shutdown: %v", err)
			}
			if err := historyStore.Save(ctx); err != nil {
				log.Printf("Failed to save history on live shutdown: %v", err)
			}
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				log.Printf("Failed to shutdown server: %v", err)
			}
		}()
	}

	scheme := "http"
	if *tlsFlag {
		scheme = "https"
			scheme = "https"
	}
	log.Printf("web-exec server starting on %s", *addr)
	displayAddr := *addr
	if strings.HasPrefix(*addr, ":") {
		displayAddr = "localhost" + *addr
			displayAddr = "localhost" + *addr
	}
	log.Printf("  Config UI: %s://%s/config", scheme, displayAddr)
	log.Printf("  Exec endpoint: %s://%s/exec/<route-path>", scheme, displayAddr)
	if *token != "" {
		log.Printf("  Auth: Bearer token required")
	} else {
		log.Printf("  Auth: No token required (open access)")
	}

	if *tlsFlag {
		cert, err := generateSelfSignedCert()
		if err != nil {
			log.Fatalf("Failed to generate self-signed certificate: %v", err)
		}
		srv.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{*cert},
			MinVersion:   tls.VersionTLS12,
		}
		log.Println("  TLS: Using auto-generated self-signed certificate")
		if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	} else {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}
}

// parseLiveDuration parses duration strings like "1d2h30m10s".
// Supports days (d), hours (h), minutes (m), seconds (s).
// Each unit can appear at most once.
func parseLiveDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration string")
	}
	s = strings.ToLower(s)
	var d time.Duration
	var seen byte
	i := 0
	for i < len(s) {
		// Parse number
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if start == i {
			return 0, fmt.Errorf("expected number at position %d", i)
		}
		n, _ := strconv.Atoi(s[start:i])
		if i >= len(s) {
			return 0, fmt.Errorf("expected unit after number")
		}
		unit := s[i]
		i++
		var unitSeen byte
		switch unit {
		case 'd':
			d += time.Duration(n) * 24 * time.Hour
			unitSeen = 1
		case 'h':
			d += time.Duration(n) * time.Hour
			unitSeen = 2
		case 'm':
			d += time.Duration(n) * time.Minute
			unitSeen = 4
		case 's':
			d += time.Duration(n) * time.Second
			unitSeen = 8
		default:
			return 0, fmt.Errorf("unknown unit '%c' at position %d", unit, i-1)
		}
		if seen&unitSeen != 0 {
			return 0, fmt.Errorf("duplicate unit '%c'", unit)
		}
		seen |= unitSeen
	}
	return d, nil
}


// generateSelfSignedCert creates an in-memory self-signed TLS certificate
// suitable for development and internal use.
func generateSelfSignedCert() (*tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate private key: %w", err)
	}

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "web-exec"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	privBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("load key pair: %w", err)
	}
	return &tlsCert, nil
}

// bearerAuth wraps a handler with Bearer token authentication.
// If token is empty, the handler is returned as-is (no auth required).
func bearerAuth(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer "+token {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// configAuth wraps the configUI handler with selective auth:
// HTML page requests (no sub-path or empty sub-path) pass through without auth,
// while API requests (paths containing "routes" or "history") require Bearer token.
// This allows the browser to load the config HTML page and show a token input field,
// then the frontend JS sends the token on subsequent API calls.
func configAuth(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip /config prefix to get the sub-path
		subPath := strings.TrimPrefix(r.URL.Path, "/config")
		subPath = strings.TrimPrefix(subPath, "/")
		// If sub-path is empty, this is the HTML page request — no auth needed
		if subPath == "" {
			next.ServeHTTP(w, r)
			return
		}
		// API routes require auth
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer "+token {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

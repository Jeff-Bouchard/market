package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata"
)

//go:embed web
var assets embed.FS

const defaultNS = "NS=seed1.emercoin.com,seed2.emercoin.com"

type Config struct {
	Listen          string  `json:"listen"`
	DataDir         string  `json:"data_dir"`
	SecureCookies   bool    `json:"secure_cookies"`
	PublicOrigin    string  `json:"public_origin"`
	Timezone        string  `json:"timezone"`
	NESSURL         string  `json:"ness_url"`
	NESSWallet      string  `json:"ness_wallet"`
	NESSAPIUser     string  `json:"ness_api_user"`
	NESSAPIPassword string  `json:"ness_api_password"`
	NESSPassword    string  `json:"ness_password"`
	EMCURL          string  `json:"emc_url"`
	EMCUser         string  `json:"emc_user"`
	EMCPassword     string  `json:"emc_password"`
	EMCWallet       *string `json:"emc_wallet"`
	ExchangeURL     string  `json:"exchange_url"`
	CoinsPerSecond  Decimal `json:"coins_per_second"`
	HoursPerSecond  int64   `json:"hours_per_second"`
	EMCPrice        Decimal `json:"emc_per_100_days"`
	NESSPrice       Decimal `json:"ness_per_100_days"`
	NCHPrice        Decimal `json:"nch_per_100_days"`
}
type App struct {
	mu           sync.Mutex
	cfg          Config
	store        *Store
	key          ed25519.PrivateKey
	zone         *time.Location
	now          func() time.Time
	client       *http.Client
	emcWalletURL string
}
type apiError struct {
	code    int
	message string
}

func (e *apiError) Error() string { return e.message }
func bad(s string) error          { return &apiError{400, s} }
func randomID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func loadKey(dir string) (ed25519.PrivateKey, error) {
	path := filepath.Join(dir, "signing.seed")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// Never replace a missing key when the installation already has state.
		if info, e := os.Stat(filepath.Join(dir, "state.jsonl")); e == nil && info.Size() > 0 {
			return nil, errors.New("signing.seed is missing from an existing installation; restore its original key")
		}
		b = make([]byte, 32)
		if _, err = rand.Read(b); err != nil {
			return nil, err
		}
		f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return nil, e
		}
		_, err = f.Write(b)
		if err == nil {
			err = f.Sync()
		}
		f.Close()
	}
	if err != nil {
		return nil, err
	}
	if len(b) != 32 {
		return nil, errors.New("signing.seed must contain exactly 32 raw bytes")
	}
	return ed25519.NewKeyFromSeed(b), nil
}
func newApp(c Config) (*App, error) {
	if c.Listen == "" {
		c.Listen = "127.0.0.1:8787"
	}
	_, listenPort, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return nil, fmt.Errorf("invalid listen address: %w", err)
	}
	portNumber, err := net.LookupPort("tcp", listenPort)
	if err != nil {
		return nil, fmt.Errorf("invalid listen port: %w", err)
	}
	if portNumber == 8080 || portNumber == 8000 {
		return nil, errors.New("portal ports 8080 and 8000 are reserved for IPFS and Skywire; set listen to 127.0.0.1:8787")
	}
	if c.DataDir == "" {
		c.DataDir = "data"
	}
	if c.Timezone == "" {
		c.Timezone = "UTC"
	}
	for _, setting := range []struct {
		amount    Decimal
		precision int
	}{{c.CoinsPerSecond, 6}, {c.EMCPrice, 8}, {c.NESSPrice, 6}, {c.NCHPrice, 0}} {
		value, err := setting.amount.units(setting.precision)
		if err != nil || value == 0 {
			return nil, errors.New("reward/prices must be positive exact amounts: NESS 6 decimals, EMC 8 decimals, NCH whole hours")
		}
	}
	rate, _ := c.CoinsPerSecond.units(6)
	if rate > 1000000 || c.HoursPerSecond <= 0 || c.HoursPerSecond > 1000000 {
		return nil, errors.New("reward settings exceed supported range")
	}
	for _, v := range []string{c.NESSURL, c.EMCURL, c.ExchangeURL, c.PublicOrigin} {
		if v != "" {
			u, e := url.Parse(v)
			if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
				return nil, errors.New("configured URLs must be absolute HTTP(S) URLs")
			}
		}
	}
	zone, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return nil, err
	}
	s, err := openStore(c.DataDir)
	if err != nil {
		return nil, err
	}
	key, err := loadKey(c.DataDir)
	if err != nil {
		s.Close()
		return nil, err
	}
	return &App{cfg: c, store: s, key: key, zone: zone, now: time.Now, client: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return errors.New("upstream redirects are not allowed")
	}}}, nil
}
func respond(w http.ResponseWriter, status int, v any) {
	if slot, ok := v.(Slot); ok {
		raw, _ := json.Marshal(slot)
		var data map[string]any
		json.Unmarshal(raw, &data)
		data["status_detail"] = slotStatus(slot)
		v = data
	}
	if data, ok := v.(map[string]string); ok && data["error"] != "" {
		detail := explainError(&apiError{status, data["error"]}, status)
		v = map[string]any{"error": detail.Message, "status_detail": detail}
	}

	if data, ok := v.(map[string]any); ok {
		if redemption, ok := data["redemption"].(Redemption); ok {
			data["status_detail"] = slotStatus(Slot{Status: redemption.State})
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func (a *App) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/faucet", a.api)
	mux.HandleFunc("/api/exchange", a.api)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]string{"status": "ok"}) })
	web, _ := fs.Sub(assets, "web")
	files := http.FileServer(http.FS(web))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		files.ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, private, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		if !a.allowedHost(r.Host) {
			http.Error(w, "Host rejected", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		mux.ServeHTTP(w, r)
	})
}

// Match Skycoin's local-host protection: a loopback service must not trust a
// browser-supplied Host/Origin pair after DNS rebinding.
func (a *App) allowedHost(host string) bool {
	if a.cfg.PublicOrigin != "" {
		u, e := url.Parse(a.cfg.PublicOrigin)
		return e == nil && strings.EqualFold(host, u.Host)
	}
	listenHost, listenPort, e := net.SplitHostPort(a.cfg.Listen)
	if e != nil {
		return false
	}
	ip := net.ParseIP(listenHost)
	if listenHost != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return true
	}
	h, p, e := net.SplitHostPort(host)
	if e != nil || p != listenPort {
		return false
	}
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip = net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
func (a *App) api(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	if r.Method != "GET" && r.Method != "POST" {
		respond(w, 405, map[string]string{"error": "Use GET or POST."})
		return
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		respond(w, 403, map[string]string{"error": "Cross-site request rejected."})
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		expected := a.cfg.PublicOrigin
		if expected == "" {
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			expected = scheme + "://" + r.Host
		}
		if origin != expected {
			respond(w, 403, map[string]string{"error": "Origin rejected."})
			return
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.store.failed {
		respond(w, 503, map[string]string{"error": "Storage requires operator review."})
		return
	}
	var id string
	if c, e := r.Cookie("privateness_session"); e == nil {
		id = c.Value
	}
	session, exists := a.store.Sessions[id]
	if !exists || session.Expires < a.now().Unix() {
		if r.Method == "POST" {
			respond(w, 403, map[string]string{"error": "Session expired. Refresh and try again."})
			return
		}
		active := 0
		for _, s := range a.store.Sessions {
			if s.Expires >= a.now().Unix() {
				active++
			}
		}
		if active >= 10000 {
			respond(w, 503, map[string]string{"error": "Service is busy. Try again later."})
			return
		}
		id = randomID(32)
		session = Session{CSRF: randomID(32), Expires: a.now().Add(24 * time.Hour).Unix()}
		if e := a.store.session(id, session); e != nil {
			respond(w, 503, map[string]string{"error": e.Error()})
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "privateness_session", Value: id, Path: "/", HttpOnly: true, Secure: a.cfg.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: 86400})
	}
	if r.Method == "GET" {
		respond(w, 200, map[string]any{"csrf": session.CSRF, "wait_seconds": int64(claimWait / time.Second), "default_ns": defaultNS, "faucet_configured": a.cfg.NESSURL != "" && a.cfg.NESSWallet != "", "nvs_configured": a.cfg.EMCURL != ""})
		return
	}
	if subtle.ConstantTimeCompare([]byte(session.CSRF), []byte(r.Header.Get("X-CSRF-Token"))) != 1 {
		respond(w, 403, map[string]string{"error": "Refresh and try again."})
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		respond(w, 415, map[string]string{"error": "Use JSON."})
		return
	}
	var input map[string]json.RawMessage
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	err := decoder.Decode(&input)
	var extra any
	if err == nil && decoder.Decode(&extra) != io.EOF {
		err = errors.New("trailing JSON")
	}
	if err != nil || input == nil {
		respond(w, 400, map[string]string{"error": "Invalid or oversized JSON request."})
		return
	}
	var result any
	status := 200
	if r.URL.Path == "/api/faucet" {
		result, status, err = a.faucet(id, session, input)
	} else {
		result, status, err = a.exchange(id, input)
	}
	if err != nil {
		var ae *apiError
		if errors.As(err, &ae) {
			status = ae.code
		} else {
			status = 503
			log.Printf("API operation failed: %v", err)
		}
		detail := explainError(err, status)
		reference := randomID(8)
		log.Printf("Request %s: %s: %v", reference, detail.Code, err)
		respond(w, status, map[string]any{"error": detail.Message, "status_detail": detail, "reference": reference})
		return
	}
	respond(w, status, result)
}
func textField(m map[string]json.RawMessage, k string, max int) (string, error) {
	var s string
	if err := json.Unmarshal(m[k], &s); err != nil || len(s) > max {
		return "", bad("Invalid " + k)
	}
	return strings.TrimSpace(s), nil
}
func boolField(m map[string]json.RawMessage, k string) bool {
	var b bool
	json.Unmarshal(m[k], &b)
	return b
}
func main() {
	path := "config.json"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Fatal("Copy config.example.json to config.json, then run again: ", err)
	}
	var c Config
	if err = json.Unmarshal(raw, &c); err != nil {
		log.Fatal(err)
	}
	app, err := newApp(c)
	if err != nil {
		log.Fatal(err)
	}
	defer app.store.Close()
	server := &http.Server{Addr: app.cfg.Listen, Handler: app.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			server.Close()
		}
	}()
	fmt.Printf("Privateness is ready at http://%s\n", app.cfg.Listen)
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Print(err)
	}
	stop()
	<-shutdownDone
	app.mu.Lock()
	defer app.mu.Unlock()
}

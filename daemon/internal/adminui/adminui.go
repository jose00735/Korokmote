// Package adminui serves the local admin page (pairing QR, devices, logs) on loopback.
package adminui

import (
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/jose00735/korokmote/daemon/internal/netinfo"
	"github.com/jose00735/korokmote/daemon/internal/pairing"
)

//go:embed index.html
var indexHTML []byte

// Backend is what the page needs from the app.
type Backend struct {
	Name         string
	Version      string
	Identity     *pairing.Identity
	Port         func() int
	Store        *pairing.Store
	Pairer       *pairing.Pairer
	PairDuration time.Duration
	Online       func() map[string]string // device id -> peer address
	Revoke       func(id string) error
	SetPerms     func(id string, perms []string) error
	Paused       func() bool
	SetPaused    func(bool)
	Autostart    func() bool
	SetAutostart func(bool) error
	Logs         func() []string
}

// UI is the running admin server.
type UI struct {
	b     Backend
	token string
	port  int
}

// Start listens on a random loopback port.
func Start(b Backend) (*UI, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("admin listen: %w", err)
	}
	var raw [24]byte
	_, _ = rand.Read(raw[:])
	u := &UI{b: b, token: base64.RawURLEncoding.EncodeToString(raw[:]), port: ln.Addr().(*net.TCPAddr).Port}
	mux := http.NewServeMux()
	mux.HandleFunc("/", u.index)
	mux.HandleFunc("/api/state", u.auth(u.state))
	mux.HandleFunc("/api/pair/open", u.auth(u.post(u.pairOpen)))
	mux.HandleFunc("/api/pair/close", u.auth(u.post(u.pairClose)))
	mux.HandleFunc("/api/device/revoke", u.auth(u.post(u.revoke)))
	mux.HandleFunc("/api/device/perms", u.auth(u.post(u.perms)))
	mux.HandleFunc("/api/pause", u.auth(u.post(u.pause)))
	mux.HandleFunc("/api/autostart", u.auth(u.post(u.autostart)))
	srv := &http.Server{Handler: secure(mux), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return u, nil
}

// URL returns the one-time login URL (sets a cookie, then redirects).
func (u *UI) URL(view string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/?k=%s#%s", u.port, u.token, view)
}

func secure(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'")
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

func (u *UI) validCookie(r *http.Request) bool {
	c, err := r.Cookie("korok")
	return err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(u.token)) == 1
}

func (u *UI) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if k := r.URL.Query().Get("k"); k != "" {
		if subtle.ConstantTimeCompare([]byte(k), []byte(u.token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "korok", Value: u.token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !u.validCookie(r) {
		http.Error(w, "Open this page from the Korokmote tray icon.", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

func (u *UI) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !u.validCookie(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h(w, r)
	}
}

func (u *UI) post(h func(r *http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if r.Method != http.MethodPost || r.Header.Get("X-Korokmote") != "1" ||
			(origin != "" && origin != fmt.Sprintf("http://127.0.0.1:%d", u.port)) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		res, err := h(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

type deviceView struct {
	pairing.Device
	Online string `json:"online,omitempty"`
}

func (u *UI) state(w http.ResponseWriter, _ *http.Request) {
	b := u.b
	st := b.Pairer.State()
	pair := map[string]any{"open": st.Open}
	hosts := netinfo.Hosts()
	if st.Open {
		payload := pairing.QRPayload(b.Identity, b.Name, hosts, b.Port(), st.Code, netinfo.PrimaryMAC())
		png, err := qrcode.Encode(payload, qrcode.Medium, 360)
		if err == nil {
			pair["qr"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
		}
		pair["short_code"] = pairing.FormatShort(st.ShortCode)
		pair["expires_in"] = int(time.Until(st.Expires).Seconds())
	}
	online := b.Online()
	var devs []deviceView
	for _, d := range b.Store.List() {
		devs = append(devs, deviceView{Device: d, Online: online[d.ID]})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name": b.Name, "version": b.Version, "id": b.Identity.ID,
		"fingerprint": b.Identity.Fingerprint, "port": b.Port(), "hosts": hosts,
		"pairing": pair, "devices": devs, "all_perms": pairing.AllPerms,
		"paused": b.Paused(), "autostart": b.Autostart(), "logs": b.Logs(),
	})
}

func (u *UI) pairOpen(*http.Request) (any, error) {
	u.b.Pairer.Open(u.b.PairDuration)
	return map[string]bool{"ok": true}, nil
}

func (u *UI) pairClose(*http.Request) (any, error) {
	u.b.Pairer.Close()
	return map[string]bool{"ok": true}, nil
}

type devReq struct {
	ID    string   `json:"id"`
	Perms []string `json:"perms"`
	On    bool     `json:"on"`
}

func readReq(r *http.Request) (devReq, error) {
	var q devReq
	err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4096)).Decode(&q)
	return q, err
}

func (u *UI) revoke(r *http.Request) (any, error) {
	q, err := readReq(r)
	if err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, u.b.Revoke(strings.TrimSpace(q.ID))
}

func (u *UI) perms(r *http.Request) (any, error) {
	q, err := readReq(r)
	if err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, u.b.SetPerms(q.ID, q.Perms)
}

func (u *UI) pause(r *http.Request) (any, error) {
	q, err := readReq(r)
	if err != nil {
		return nil, err
	}
	u.b.SetPaused(q.On)
	return map[string]bool{"ok": true}, nil
}

func (u *UI) autostart(r *http.Request) (any, error) {
	q, err := readReq(r)
	if err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, u.b.SetAutostart(q.On)
}

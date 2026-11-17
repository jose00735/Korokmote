package pairing

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jose00735/korokmote/daemon/internal/proto"
)

// MaxFailures closes a pairing window after this many bad codes.
const MaxFailures = 5

// ErrNoWindow is returned when no pairing window is open.
var ErrNoWindow = errors.New("no pairing window open")

// ErrBadCode is returned for a wrong code.
var ErrBadCode = errors.New("invalid pairing code")

// shortAlphabet avoids look-alike characters.
const shortAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// WindowState describes the current pairing window.
type WindowState struct {
	Open      bool      `json:"open"`
	Code      string    `json:"-"`
	ShortCode string    `json:"short_code,omitempty"`
	Expires   time.Time `json:"expires"`
}

// Pairer manages pairing windows.
type Pairer struct {
	mu       sync.Mutex
	state    WindowState
	failures int
	store    *Store
	timer    *time.Timer
	onChange func(open bool)
	onPaired func(Device)
}

// NewPairer creates a pairing manager bound to a device store.
func NewPairer(store *Store) *Pairer { return &Pairer{store: store} }

// OnChange registers a callback for window open/close.
func (p *Pairer) OnChange(f func(open bool)) { p.onChange = f }

// OnPaired registers a callback for a successful pairing.
func (p *Pairer) OnPaired(f func(Device)) { p.onPaired = f }

// Open starts (or restarts) a pairing window.
func (p *Pairer) Open(d time.Duration) WindowState {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	p.mu.Lock()
	p.state = WindowState{
		Open:      true,
		Code:      base64.RawURLEncoding.EncodeToString(raw[:]),
		ShortCode: randomShort(8),
		Expires:   time.Now().Add(d),
	}
	p.failures = 0
	if p.timer != nil {
		p.timer.Stop()
	}
	p.timer = time.AfterFunc(d, p.Close)
	st := p.state
	cb := p.onChange
	p.mu.Unlock()
	if cb != nil {
		cb(true)
	}
	return st
}

// Close ends the pairing window.
func (p *Pairer) Close() {
	p.mu.Lock()
	was := p.state.Open
	p.state = WindowState{}
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	cb := p.onChange
	p.mu.Unlock()
	if was && cb != nil {
		cb(false)
	}
}

// State returns the current window.
func (p *Pairer) State() WindowState {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state.Open && time.Now().After(p.state.Expires) {
		return WindowState{}
	}
	return p.state
}

// TryPair validates a code and registers the device.
func (p *Pairer) TryPair(code string, info proto.DeviceInfo) (string, *Device, error) {
	p.mu.Lock()
	st := p.state
	if !st.Open || time.Now().After(st.Expires) {
		p.mu.Unlock()
		return "", nil, ErrNoWindow
	}
	normShort := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(code))
	longOK := subtle.ConstantTimeCompare([]byte(code), []byte(st.Code)) == 1
	shortOK := subtle.ConstantTimeCompare([]byte(normShort), []byte(st.ShortCode)) == 1
	if !longOK && !shortOK {
		p.failures++
		closeNow := p.failures >= MaxFailures
		p.mu.Unlock()
		if closeNow {
			p.Close()
		}
		return "", nil, ErrBadCode
	}
	p.mu.Unlock()
	p.Close()
	tok, dev, err := p.store.Add(info)
	if err != nil {
		return "", nil, err
	}
	if p.onPaired != nil {
		p.onPaired(*dev)
	}
	return tok, dev, nil
}

func randomShort(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = shortAlphabet[int(b[i])%len(shortAlphabet)]
	}
	return string(b)
}

// FormatShort groups a short code as XXXX-XXXX.
func FormatShort(s string) string {
	if len(s) == 8 {
		return s[:4] + "-" + s[4:]
	}
	return s
}

// QRPayload builds the korokmote://pair URI described in PROTOCOL.md.
func QRPayload(id *Identity, name string, hosts []string, port int, code, mac string) string {
	q := url.Values{}
	q.Set("v", strconv.Itoa(proto.Version))
	q.Set("id", id.ID)
	q.Set("name", name)
	q.Set("h", strings.Join(hosts, ","))
	q.Set("p", strconv.Itoa(port))
	q.Set("fp", id.Fingerprint)
	q.Set("c", code)
	if mac != "" {
		q.Set("mac", mac)
	}
	return "korokmote://pair?" + q.Encode()
}

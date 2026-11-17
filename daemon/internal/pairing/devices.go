package pairing

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/jose00735/korokmote/daemon/internal/proto"
)

// Permissions a device can hold.
const (
	PermInput     = "input"
	PermMedia     = "media"
	PermAudio     = "audio"
	PermPower     = "power"
	PermFiles     = "files"
	PermLauncher  = "launcher"
	PermClipboard = "clipboard"
)

// AllPerms lists every permission in display order.
var AllPerms = []string{PermInput, PermMedia, PermAudio, PermPower, PermFiles, PermLauncher, PermClipboard}

// DefaultPerms is what a newly paired device receives.
var DefaultPerms = []string{PermInput, PermMedia, PermAudio, PermPower, PermFiles, PermLauncher, PermClipboard}

// Device is a paired phone.
type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Platform  string    `json:"platform"`
	App       string    `json:"app"`
	TokenHash string    `json:"token_hash"`
	Perms     []string  `json:"perms"`
	Paired    time.Time `json:"paired"`
	LastSeen  time.Time `json:"last_seen"`
}

// Has reports whether the device holds perm.
func (d *Device) Has(perm string) bool { return slices.Contains(d.Perms, perm) }

// Store persists paired devices in devices.json.
type Store struct {
	mu      sync.Mutex
	path    string
	devices map[string]*Device
}

// OpenStore loads the device store.
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, devices: map[string]*Device{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read devices: %w", err)
	}
	var list []*Device
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("parse devices: %w", err)
	}
	for _, d := range list {
		s.devices[d.ID] = d
	}
	return s, nil
}

func (s *Store) saveLocked() error {
	list := make([]*Device, 0, len(s.devices))
	for _, d := range s.devices {
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Paired.Before(list[j].Paired) })
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write devices: %w", err)
	}
	return os.Rename(tmp, s.path)
}

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

// Add registers (or re-registers) a device and returns its new token.
func (s *Store) Add(info proto.DeviceInfo) (string, *Device, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", nil, fmt.Errorf("token: %w", err)
	}
	tok := base64.RawURLEncoding.EncodeToString(raw[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	d := &Device{
		ID: info.ID, Name: info.Name, Platform: info.Platform, App: info.App,
		TokenHash: hashToken(tok), Perms: slices.Clone(DefaultPerms),
		Paired: time.Now(), LastSeen: time.Now(),
	}
	if old, ok := s.devices[info.ID]; ok {
		d.Perms = old.Perms
	}
	s.devices[d.ID] = d
	if err := s.saveLocked(); err != nil {
		return "", nil, err
	}
	cp := *d
	return tok, &cp, nil
}

// Authenticate checks a device token in constant time.
func (s *Store) Authenticate(id, tok string) (*Device, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[id]
	want := hashToken(tok)
	if !ok {
		// Compare anyway to keep timing uniform.
		subtle.ConstantTimeCompare([]byte(want), []byte(hashToken("")))
		return nil, false
	}
	if subtle.ConstantTimeCompare([]byte(want), []byte(d.TokenHash)) != 1 {
		return nil, false
	}
	d.LastSeen = time.Now()
	_ = s.saveLocked()
	cp := *d
	cp.Perms = slices.Clone(d.Perms)
	return &cp, true
}

// Get returns a copy of a device.
func (s *Store) Get(id string) (*Device, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[id]
	if !ok {
		return nil, false
	}
	cp := *d
	cp.Perms = slices.Clone(d.Perms)
	return &cp, true
}

// List returns copies of all devices, oldest pairing first.
func (s *Store) List() []Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Device, 0, len(s.devices))
	for _, d := range s.devices {
		cp := *d
		cp.Perms = slices.Clone(d.Perms)
		cp.TokenHash = ""
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Paired.Before(out[j].Paired) })
	return out
}

// Revoke removes a device.
func (s *Store) Revoke(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[id]; !ok {
		return fmt.Errorf("device %s not found", id)
	}
	delete(s.devices, id)
	return s.saveLocked()
}

// SetPerms replaces a device's permissions.
func (s *Store) SetPerms(id string, perms []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[id]
	if !ok {
		return fmt.Errorf("device %s not found", id)
	}
	clean := []string{}
	for _, p := range AllPerms {
		if slices.Contains(perms, p) {
			clean = append(clean, p)
		}
	}
	d.Perms = clean
	return s.saveLocked()
}

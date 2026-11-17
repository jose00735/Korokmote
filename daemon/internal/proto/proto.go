// Package proto defines the Korokmote wire protocol, version 1.
// See docs/PROTOCOL.md for the specification.
package proto

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Version is the highest protocol version this build speaks.
const Version = 1

// DefaultPort is the daemon's default TCP port; BeaconPort is the UDP discovery port.
const (
	DefaultPort = 47810
	BeaconPort  = 47811
	ServiceType = "_korokmote._tcp"
)

// Envelope is the JSON frame wrapper.
type Envelope struct {
	T  string          `json:"t"`
	ID *int64          `json:"id,omitempty"`
	Re *int64          `json:"re,omitempty"`
	D  json.RawMessage `json:"d,omitempty"`
}

// Error codes.
const (
	ErrUnauthorized = "unauthorized"
	ErrForbidden    = "forbidden"
	ErrBadRequest   = "bad_request"
	ErrUnsupported  = "unsupported"
	ErrNotFound     = "not_found"
	ErrLocked       = "locked"
	ErrRateLimited  = "rate_limited"
	ErrInternal     = "internal"
)

// Error is a protocol-level error returned to the client.
type Error struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Msg }

// Errorf builds a protocol error.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// AsError converts any error into a protocol error.
func AsError(err error) *Error {
	var pe *Error
	if errors.As(err, &pe) {
		return pe
	}
	return &Error{Code: ErrInternal, Msg: err.Error()}
}

// Encode builds a message with an optional request id.
func Encode(t string, id *int64, d any) ([]byte, error) {
	env := Envelope{T: t, ID: id}
	if d != nil {
		raw, err := json.Marshal(d)
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", t, err)
		}
		env.D = raw
	}
	return json.Marshal(env)
}

// Reply builds an ok or err reply to request re.
func Reply(re int64, d any, err error) []byte {
	env := Envelope{Re: &re}
	if err != nil {
		env.T = "err"
		env.D, _ = json.Marshal(AsError(err))
	} else {
		env.T = "ok"
		if d == nil {
			d = struct{}{}
		}
		raw, mErr := json.Marshal(d)
		if mErr != nil {
			env.T = "err"
			raw, _ = json.Marshal(&Error{Code: ErrInternal, Msg: mErr.Error()})
		}
		env.D = raw
	}
	b, _ := json.Marshal(env)
	return b
}

// Event builds a server event (no id).
func Event(t string, d any) []byte {
	b, _ := Encode(t, nil, d)
	return b
}

// ---- Payloads ----

type Hello struct {
	V      int    `json:"v"`
	Device string `json:"device"`
	Token  string `json:"token"`
	App    string `json:"app"`
}

type DaemonInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	OS      string `json:"os,omitempty"`
	Version string `json:"version,omitempty"`
}

type Screen struct {
	W int `json:"w"`
	H int `json:"h"`
}

type HelloReply struct {
	V      int        `json:"v"`
	Daemon DaemonInfo `json:"daemon"`
	Caps   []string   `json:"caps"`
	Perms  []string   `json:"perms"`
	Screen Screen     `json:"screen"`
	Locked bool       `json:"locked"`
}

type Ping struct {
	TS int64 `json:"ts"`
}

type Key struct {
	K string `json:"k"`
	A string `json:"a"`
}

type Combo struct {
	Keys []string `json:"keys"`
}

type Text struct {
	S string `json:"s"`
}

type Click struct {
	B int `json:"b"`
	N int `json:"n"`
}

type Media struct {
	A string `json:"a"`
}

type AudioDevice struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Default bool    `json:"default"`
	Level   float64 `json:"level"`
	Muted   bool    `json:"muted"`
}

type AudioSet struct {
	Device string   `json:"device"`
	Level  *float64 `json:"level,omitempty"`
	Delta  *float64 `json:"delta,omitempty"`
}

type AudioMute struct {
	Device string `json:"device"`
	Muted  *bool  `json:"muted,omitempty"`
}

type AudioEvent struct {
	Device string  `json:"device"`
	Level  float64 `json:"level"`
	Muted  bool    `json:"muted"`
}

type Power struct {
	A     string `json:"a"`
	Delay int    `json:"delay,omitempty"`
}

type WindowInfo struct {
	H     uint64 `json:"h"`
	Title string `json:"title"`
	Exe   string `json:"exe"`
}

type WinRef struct {
	H uint64 `json:"h"`
}

type AppInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type AppRef struct {
	ID string `json:"id"`
}

type FsEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Dir   bool   `json:"dir"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
}

type FsPath struct {
	Path string `json:"path"`
}

type ClipText struct {
	Text string `json:"text"`
}

type ClipSub struct {
	On bool `json:"on"`
}

type RemoteRef struct {
	ID string `json:"id"`
}

type RemoteSummary struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Icon  string   `json:"icon,omitempty"`
	Match []string `json:"match,omitempty"`
}

type RemoteAct struct {
	Remote string            `json:"remote"`
	Action string            `json:"action"`
	Args   map[string]string `json:"args,omitempty"`
}

type LockEvent struct {
	Locked bool `json:"locked"`
}

type ForegroundEvent struct {
	Title  string `json:"title"`
	Exe    string `json:"exe"`
	Remote string `json:"remote,omitempty"`
}

type Bye struct {
	Reason string `json:"reason"`
}

// PairRequest is the body of POST /v1/pair.
type PairRequest struct {
	Code   string     `json:"code"`
	Device DeviceInfo `json:"device"`
}

type DeviceInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	App      string `json:"app"`
}

type PairResponse struct {
	Token  string     `json:"token"`
	Daemon DaemonInfo `json:"daemon"`
	Phrase string     `json:"phrase"`
}

// BeaconReply is the JSON answer to a UDP discovery probe.
type BeaconReply struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	V    int    `json:"v"`
	FP   string `json:"fp"`
	Pair bool   `json:"pair"`
	Port int    `json:"port"`
}

// BeaconProbe is the 8-byte UDP discovery request.
var BeaconProbe = []byte{'K', 'R', 'K', 'M', '?', Version, 0, 0}

// ---- Verification phrase ----

//go:embed wordlist.txt
var wordlistRaw string

// Wordlist holds the 256 words used for verification phrases.
var Wordlist = strings.Fields(wordlistRaw)

// Phrase derives the three-word verification phrase from the certificate
// fingerprint (lowercase hex) and the device id: SHA-256(fp || deviceID).
func Phrase(fpHex, deviceID string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(fpHex) + deviceID))
	return Wordlist[sum[0]] + "-" + Wordlist[sum[1]] + "-" + Wordlist[sum[2]]
}

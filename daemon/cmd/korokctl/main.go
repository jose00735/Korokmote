// Command korokctl is a command-line controller for development, testing and scripting.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jose00735/korokmote/daemon/internal/client"
	"github.com/jose00735/korokmote/daemon/internal/pairing"
	"github.com/jose00735/korokmote/daemon/internal/proto"
	"github.com/jose00735/korokmote/daemon/internal/remotes"
)

var version = "dev"

type saved struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	FP       string `json:"fp"`
	DeviceID string `json:"device_id"`
	Token    string `json:"token"`
	Name     string `json:"name"`
}

func statePath() string {
	if p := os.Getenv("KOROKCTL_STATE"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".korokctl.json")
}

func load() (saved, error) {
	var s saved
	b, err := os.ReadFile(statePath())
	if err != nil {
		return s, errors.New("not paired: run korokctl pair first")
	}
	return s, json.Unmarshal(b, &s)
}

func usage() {
	fmt.Fprint(os.Stderr, `korokctl `+version+` — Korokmote command-line controller

  korokctl discover
  korokctl pair --uri 'korokmote://pair?...'
  korokctl pair --host 192.168.1.20 [--port 47810] --code ABCD-EFGH
  korokctl send <type> [json]          e.g. send media '{"a":"playpause"}'
  korokctl key <NAME>                  e.g. key ENTER
  korokctl text "<string>"
  korokctl move <dx> <dy>
  korokctl bench [--n 200]
  korokctl remotes lint <file.yaml>
`)
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var err error
	switch os.Args[1] {
	case "discover":
		err = discover()
	case "pair":
		err = pair(ctx, os.Args[2:])
	case "send":
		if len(os.Args) < 3 {
			usage()
		}
		payload := "{}"
		if len(os.Args) > 3 {
			payload = os.Args[3]
		}
		err = send(ctx, os.Args[2], json.RawMessage(payload))
	case "key":
		if len(os.Args) < 3 {
			usage()
		}
		err = send(ctx, "key", proto.Key{K: os.Args[2], A: "tap"})
	case "text":
		if len(os.Args) < 3 {
			usage()
		}
		err = send(ctx, "text", proto.Text{S: os.Args[2]})
	case "move":
		if len(os.Args) < 4 {
			usage()
		}
		err = move(ctx, os.Args[2], os.Args[3])
	case "bench":
		err = bench(ctx, os.Args[2:])
	case "remotes":
		if len(os.Args) < 4 || os.Args[2] != "lint" {
			usage()
		}
		err = lint(os.Args[3])
	case "version", "--version":
		fmt.Println("korokctl", version)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func discover() error {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return err
	}
	defer c.Close()
	targets := []string{"255.255.255.255", "127.0.0.1"}
	for _, t := range targets {
		_, _ = c.WriteToUDP(proto.BeaconProbe, &net.UDPAddr{IP: net.ParseIP(t), Port: proto.BeaconPort})
	}
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	seen := map[string]bool{}
	buf := make([]byte, 2048)
	for {
		n, src, err := c.ReadFromUDP(buf)
		if err != nil {
			break
		}
		var r proto.BeaconReply
		if json.Unmarshal(buf[:n], &r) != nil || seen[r.ID+src.IP.String()] {
			continue
		}
		seen[r.ID+src.IP.String()] = true
		pair := ""
		if r.Pair {
			pair = " [pairing open]"
		}
		fmt.Printf("%s  %s:%d  id=%s fp=%s%s\n", r.Name, src.IP, r.Port, r.ID, r.FP, pair)
	}
	if len(seen) == 0 {
		fmt.Println("no daemons found")
	}
	return nil
}

func pair(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	uri := fs.String("uri", "", "QR payload")
	host := fs.String("host", "", "daemon address")
	port := fs.Int("port", proto.DefaultPort, "daemon port")
	code := fs.String("code", "", "pairing code")
	name := fs.String("name", "korokctl", "device name")
	_ = fs.Parse(args)
	fp := ""
	if *uri != "" {
		u, err := url.Parse(*uri)
		if err != nil || u.Scheme != "korokmote" {
			return errors.New("bad pairing URI")
		}
		q := u.Query()
		*host = strings.Split(q.Get("h"), ",")[0]
		*port, _ = strconv.Atoi(q.Get("p"))
		*code = q.Get("c")
		fp = q.Get("fp")
	}
	if *host == "" || *code == "" {
		return errors.New("need --uri, or --host and --code")
	}
	devID := pairing.NewUUID()
	resp, seen, err := client.Pair(ctx, *host, *port, fp, *code, proto.DeviceInfo{ID: devID, Name: *name, Platform: "cli", App: version})
	if err != nil {
		return err
	}
	phrase := proto.Phrase(seen, devID)
	if phrase != resp.Phrase {
		return fmt.Errorf("verification phrase mismatch: possible interception")
	}
	fmt.Printf("paired with %s\nverification phrase: %s\n", resp.Daemon.Name, phrase)
	if fp == "" {
		fmt.Println("(manual pairing: check that the phrase matches the PC)")
	}
	s := saved{Host: *host, Port: *port, FP: seen, DeviceID: devID, Token: resp.Token, Name: resp.Daemon.Name}
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(statePath(), b, 0o600)
}

func connect(ctx context.Context) (*client.Client, error) {
	s, err := load()
	if err != nil {
		return nil, err
	}
	return client.Dial(ctx, s.Host, s.Port, s.FP, s.DeviceID, s.Token)
}

func send(ctx context.Context, t string, d any) error {
	c, err := connect(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	res, err := c.Request(ctx, t, d)
	if err != nil {
		return err
	}
	var pretty any
	_ = json.Unmarshal(res, &pretty)
	out, _ := json.MarshalIndent(pretty, "", "  ")
	fmt.Println(string(out))
	return nil
}

func move(ctx context.Context, sx, sy string) error {
	dx, err1 := strconv.Atoi(sx)
	dy, err2 := strconv.Atoi(sy)
	if err1 != nil || err2 != nil {
		return errors.New("dx and dy must be integers")
	}
	c, err := connect(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Binary(ctx, proto.EncodeRel(proto.OpMove, int16(dx), int16(dy), 0)); err != nil {
		return err
	}
	_, err = c.Ping(ctx) // flush
	return err
}

func bench(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	n := fs.Int("n", 200, "round trips")
	_ = fs.Parse(args)
	c, err := connect(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	var ds []time.Duration
	for i := 0; i < *n; i++ {
		d, err := c.Ping(ctx)
		if err != nil {
			return err
		}
		ds = append(ds, d)
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	pct := func(p float64) time.Duration { return ds[int(float64(len(ds)-1)*p)] }
	fmt.Printf("round trips: %d  p50=%s  p90=%s  p99=%s  max=%s\n", len(ds), pct(.5), pct(.9), pct(.99), ds[len(ds)-1])
	return nil
}

func lint(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	r, err := remotes.Parse(b, path)
	if err != nil {
		return err
	}
	fmt.Printf("ok: %s (%s), %d actions\n", r.ID, r.Name, len(r.Actions))
	return nil
}

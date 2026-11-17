package discovery

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jose00735/korokmote/daemon/internal/netinfo"
	"github.com/jose00735/korokmote/daemon/internal/proto"
)

var mdnsGroup = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

// Info is what gets advertised.
type Info struct {
	ID       string
	Name     string
	Port     int
	FPPrefix string
	Pairing  func() bool
}

// Advertiser runs the mDNS responder and the UDP beacon.
type Advertiser struct {
	info     Info
	instance string
	host     string
	service  string
	conn     *net.UDPConn
	beacon   *net.UDPConn
	mu       sync.Mutex
	senders  map[string]*net.UDPConn // by local IP
	stop     chan struct{}
}

var unsafeLabel = regexp.MustCompile(`[^A-Za-z0-9 _\-()]+`)

// Start begins advertising. Failures of one mechanism are logged, not fatal.
func Start(info Info) *Advertiser {
	inst := strings.TrimSpace(unsafeLabel.ReplaceAllString(info.Name, ""))
	if inst == "" {
		inst = "Korokmote"
	}
	a := &Advertiser{
		info:     info,
		service:  proto.ServiceType + ".local.",
		instance: inst + "." + proto.ServiceType + ".local.",
		host:     "korokmote-" + strings.ReplaceAll(info.ID, "-", "")[:8] + ".local.",
		senders:  map[string]*net.UDPConn{},
		stop:     make(chan struct{}),
	}
	if err := a.startMDNS(); err != nil {
		slog.Warn("mDNS unavailable", "err", err)
	}
	if err := a.startBeacon(); err != nil {
		slog.Warn("UDP beacon unavailable", "err", err)
	}
	return a
}

func (a *Advertiser) startMDNS() error {
	ifs := netinfo.Interfaces()
	var first *net.Interface
	if len(ifs) > 0 {
		if it, err := net.InterfaceByIndex(ifs[0].Index); err == nil {
			first = it
		}
	}
	c, err := net.ListenMulticastUDP("udp4", first, mdnsGroup)
	if err != nil {
		return fmt.Errorf("listen 5353: %w", err)
	}
	a.conn = c
	for _, it := range ifs[min(1, len(ifs)):] {
		if err := joinGroup(c, mdnsGroup.IP, it.IP); err != nil {
			slog.Debug("mDNS join failed", "iface", it.Name, "err", err)
		}
	}
	go a.readLoop()
	go func() {
		// Announce on start (RFC 6762 section 8.3) and again after one second.
		a.Announce()
		select {
		case <-time.After(time.Second):
			a.Announce()
		case <-a.stop:
		}
	}()
	return nil
}

func (a *Advertiser) txt() []string {
	pair := "0"
	if a.info.Pairing != nil && a.info.Pairing() {
		pair = "1"
	}
	return []string{
		"id=" + a.info.ID,
		"name=" + a.info.Name,
		"v=" + strconv.Itoa(proto.Version),
		"fp=" + a.info.FPPrefix,
		"pair=" + pair,
	}
}

func (a *Advertiser) records(ip net.IP, ttl uint32) (ptr record, extras []record) {
	ptr = record{name: a.service, rtype: typePTR, ttl: ttl, data: ptrData(a.instance)}
	extras = []record{
		{name: a.instance, rtype: typeSRV, flush: true, ttl: ttl, data: srvData(a.info.Port, a.host)},
		{name: a.instance, rtype: typeTXT, flush: true, ttl: ttl, data: txtData(a.txt())},
		{name: a.host, rtype: typeA, flush: true, ttl: ttl, data: aData(ip)},
	}
	return
}

func (a *Advertiser) sender(local net.IP) (*net.UDPConn, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c, ok := a.senders[local.String()]; ok {
		return c, nil
	}
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: local})
	if err != nil {
		return nil, err
	}
	if err := setMulticastIface(c, local); err != nil {
		slog.Debug("IP_MULTICAST_IF failed", "ip", local, "err", err)
	}
	a.senders[local.String()] = c
	return c, nil
}

func (a *Advertiser) sendOn(iface netinfo.Iface, msg []byte, unicastTo *net.UDPAddr) {
	c, err := a.sender(iface.IP)
	if err != nil {
		slog.Debug("mDNS sender", "err", err)
		return
	}
	dst := mdnsGroup
	if unicastTo != nil {
		dst = unicastTo
	}
	if _, err := c.WriteToUDP(msg, dst); err != nil {
		slog.Debug("mDNS send", "err", err)
	}
}

// Announce multicasts the full record set on every interface.
func (a *Advertiser) Announce() { a.announce(120) }

func (a *Advertiser) announce(ttl uint32) {
	for _, it := range netinfo.Interfaces() {
		ptr, extras := a.records(it.IP, ttl)
		a.sendOn(it, buildResponse(append([]record{ptr}, extras...), nil), nil)
	}
}

func (a *Advertiser) readLoop() {
	buf := make([]byte, 9000)
	for {
		n, src, err := a.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-a.stop:
				return
			default:
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		qs, err := parseQuery(buf[:n])
		if err != nil || len(qs) == 0 {
			continue
		}
		a.answer(qs, src)
	}
}

func (a *Advertiser) answer(qs []question, src *net.UDPAddr) {
	iface, ok := netinfo.ForPeer(src.IP)
	if !ok {
		return
	}
	ptr, extras := a.records(iface.IP, 120)
	srv, txt, arec := extras[0], extras[1], extras[2]
	var answers []record
	add := func(r record) {
		for _, x := range answers {
			if x.name == r.name && x.rtype == r.rtype {
				return
			}
		}
		answers = append(answers, r)
	}
	inst := strings.ToLower(a.instance)
	host := strings.ToLower(a.host)
	for _, q := range qs {
		switch {
		case q.name == "_services._dns-sd._udp.local." && (q.qtype == typePTR || q.qtype == typeANY):
			add(record{name: "_services._dns-sd._udp.local.", rtype: typePTR, ttl: 120, data: ptrData(a.service)})
		case q.name == strings.ToLower(a.service) && (q.qtype == typePTR || q.qtype == typeANY):
			add(ptr)
		case q.name == inst && (q.qtype == typeSRV || q.qtype == typeANY):
			add(srv)
		case q.name == inst && q.qtype == typeTXT:
			add(txt)
		case q.name == host && (q.qtype == typeA || q.qtype == typeANY):
			add(arec)
		}
	}
	if len(answers) == 0 {
		return
	}
	// Additional records so one round trip resolves everything.
	var more []record
	for _, r := range []record{srv, txt, arec} {
		dup := false
		for _, x := range answers {
			if x.name == r.name && x.rtype == r.rtype {
				dup = true
			}
		}
		if !dup {
			more = append(more, r)
		}
	}
	msg := buildResponse(answers, more)
	if src.Port != 5353 {
		// Legacy unicast query (RFC 6762 section 6.7): reply directly.
		a.sendOn(iface, msg, src)
		return
	}
	a.sendOn(iface, msg, nil)
}

func (a *Advertiser) startBeacon() error {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{Port: proto.BeaconPort})
	if err != nil {
		return err
	}
	a.beacon = c
	go func() {
		buf := make([]byte, 64)
		for {
			n, src, err := c.ReadFromUDP(buf)
			if err != nil {
				select {
				case <-a.stop:
					return
				default:
				}
				time.Sleep(100 * time.Millisecond)
				continue
			}
			if n < 5 || string(buf[:5]) != "KRKM?" || !netinfo.IsLocalPeer(src.IP) {
				continue
			}
			reply := proto.BeaconReply{
				ID: a.info.ID, Name: a.info.Name, V: proto.Version, FP: a.info.FPPrefix,
				Pair: a.info.Pairing != nil && a.info.Pairing(), Port: a.info.Port,
			}
			b, _ := json.Marshal(reply)
			_, _ = c.WriteToUDP(b, src)
		}
	}()
	return nil
}

// Stop sends goodbye packets and closes sockets.
func (a *Advertiser) Stop() {
	select {
	case <-a.stop:
		return
	default:
	}
	if a.conn != nil {
		a.announce(0)
	}
	close(a.stop)
	if a.conn != nil {
		_ = a.conn.Close()
	}
	if a.beacon != nil {
		_ = a.beacon.Close()
	}
	a.mu.Lock()
	for _, c := range a.senders {
		_ = c.Close()
	}
	a.mu.Unlock()
}

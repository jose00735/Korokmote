// Package netinfo finds the LAN addresses and MAC the daemon advertises.
package netinfo

import (
	"net"
	"sort"
	"strings"
)

// Iface is a usable IPv4 interface.
type Iface struct {
	Name  string
	Index int
	IP    net.IP
	Net   *net.IPNet
	MAC   string
}

var virtualHints = []string{"vmware", "virtualbox", "vethernet", "hyper-v", "docker", "wsl", "loopback", "tailscale", "zerotier", "vpn", "tap", "tun"}

func isVirtual(name string) bool {
	n := strings.ToLower(name)
	for _, h := range virtualHints {
		if strings.Contains(n, h) {
			return true
		}
	}
	return false
}

// Interfaces returns up, non-loopback IPv4 interfaces, physical-looking ones first.
func Interfaces() []Iface {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []Iface
	for _, it := range ifs {
		if it.Flags&net.FlagUp == 0 || it.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := it.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipn.IP.To4()
			if ip4 == nil || ip4.IsLinkLocalUnicast() {
				continue
			}
			out = append(out, Iface{Name: it.Name, Index: it.Index, IP: ip4, Net: ipn, MAC: it.HardwareAddr.String()})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		vi, vj := isVirtual(out[i].Name), isVirtual(out[j].Name)
		if vi != vj {
			return !vi
		}
		pi, pj := out[i].IP.IsPrivate(), out[j].IP.IsPrivate()
		if pi != pj {
			return pi
		}
		return false
	})
	return out
}

// Hosts returns advertised IPv4 addresses as strings (best first, max 4).
func Hosts() []string {
	var out []string
	for _, i := range Interfaces() {
		out = append(out, i.IP.String())
		if len(out) == 4 {
			break
		}
	}
	return out
}

// PrimaryMAC returns the MAC of the best interface.
func PrimaryMAC() string {
	for _, i := range Interfaces() {
		if i.MAC != "" {
			return i.MAC
		}
	}
	return ""
}

// ForPeer returns the local interface on the same subnet as peer, if any.
func ForPeer(peer net.IP) (Iface, bool) {
	for _, i := range Interfaces() {
		if i.Net.Contains(peer) {
			return i, true
		}
	}
	return Iface{}, false
}

// IsLocalPeer reports whether a peer address is loopback, private or link-local
// (or CGNAT 100.64/10, which Tailscale uses).
func IsLocalPeer(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	return cgnat.Contains(ip)
}

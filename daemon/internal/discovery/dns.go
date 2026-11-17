// Package discovery advertises the daemon on the LAN via mDNS/DNS-SD and a UDP beacon.
package discovery

import (
	"encoding/binary"
	"errors"
	"net"
	"strings"
)

// Minimal DNS message encoding/decoding for mDNS (RFC 1035 / 6762).

const (
	typeA   = 1
	typePTR = 12
	typeTXT = 16
	typeSRV = 33
	typeANY = 255

	classIN    = 1
	cacheFlush = 0x8000
)

type question struct {
	name  string
	qtype uint16
}

type record struct {
	name  string
	rtype uint16
	flush bool
	ttl   uint32
	data  []byte
}

var errShort = errors.New("short dns message")

// readName decodes a (possibly compressed) domain name at off.
func readName(msg []byte, off int) (string, int, error) {
	var labels []string
	end := -1
	for hops := 0; hops < 64; hops++ {
		if off >= len(msg) {
			return "", 0, errShort
		}
		l := int(msg[off])
		switch {
		case l == 0:
			if end < 0 {
				end = off + 1
			}
			return strings.Join(labels, ".") + ".", end, nil
		case l&0xC0 == 0xC0:
			if off+1 >= len(msg) {
				return "", 0, errShort
			}
			if end < 0 {
				end = off + 2
			}
			off = int(binary.BigEndian.Uint16(msg[off:]) & 0x3FFF)
		default:
			off++
			if off+l > len(msg) {
				return "", 0, errShort
			}
			labels = append(labels, string(msg[off:off+l]))
			off += l
		}
	}
	return "", 0, errors.New("dns name loop")
}

// parseQuery returns the questions of a query message (nil for responses).
func parseQuery(msg []byte) ([]question, error) {
	if len(msg) < 12 {
		return nil, errShort
	}
	flags := binary.BigEndian.Uint16(msg[2:])
	if flags&0x8000 != 0 { // response
		return nil, nil
	}
	qd := int(binary.BigEndian.Uint16(msg[4:]))
	off := 12
	var qs []question
	for i := 0; i < qd; i++ {
		name, next, err := readName(msg, off)
		if err != nil {
			return qs, err
		}
		if next+4 > len(msg) {
			return qs, errShort
		}
		qs = append(qs, question{name: strings.ToLower(name), qtype: binary.BigEndian.Uint16(msg[next:])})
		off = next + 4
	}
	return qs, nil
}

func appendName(b []byte, name string) []byte {
	name = strings.TrimSuffix(name, ".")
	for _, l := range strings.Split(name, ".") {
		if len(l) > 63 {
			l = l[:63]
		}
		b = append(b, byte(len(l)))
		b = append(b, l...)
	}
	return append(b, 0)
}

func encodeName(name string) []byte { return appendName(nil, name) }

func ptrData(target string) []byte { return encodeName(target) }

func srvData(port int, target string) []byte {
	b := make([]byte, 6)
	binary.BigEndian.PutUint16(b[4:], uint16(port))
	return appendName(b, target)
}

func txtData(kv []string) []byte {
	var b []byte
	for _, s := range kv {
		if len(s) > 255 {
			s = s[:255]
		}
		b = append(b, byte(len(s)))
		b = append(b, s...)
	}
	if len(b) == 0 {
		b = []byte{0}
	}
	return b
}

func aData(ip net.IP) []byte { return []byte(ip.To4()) }

// buildResponse encodes an authoritative mDNS response.
func buildResponse(answers, extras []record) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint16(b[2:], 0x8400)
	binary.BigEndian.PutUint16(b[6:], uint16(len(answers)))
	binary.BigEndian.PutUint16(b[10:], uint16(len(extras)))
	for _, r := range append(append([]record{}, answers...), extras...) {
		b = appendName(b, r.name)
		var hdr [10]byte
		binary.BigEndian.PutUint16(hdr[0:], r.rtype)
		class := uint16(classIN)
		if r.flush {
			class |= cacheFlush
		}
		binary.BigEndian.PutUint16(hdr[2:], class)
		binary.BigEndian.PutUint32(hdr[4:], r.ttl)
		binary.BigEndian.PutUint16(hdr[8:], uint16(len(r.data)))
		b = append(b, hdr[:]...)
		b = append(b, r.data...)
	}
	return b
}

package discovery

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestQueryParseAndResponse(t *testing.T) {
	q := make([]byte, 12)
	binary.BigEndian.PutUint16(q[4:], 1)
	q = appendName(q, "_korokmote._tcp.local.")
	q = append(q, 0, typePTR, 0, 1)
	qs, err := parseQuery(q)
	if err != nil || len(qs) != 1 || qs[0].name != "_korokmote._tcp.local." || qs[0].qtype != typePTR {
		t.Fatalf("parse: %+v %v", qs, err)
	}
	resp := buildResponse([]record{{name: "_korokmote._tcp.local.", rtype: typePTR, ttl: 120, data: ptrData("PC._korokmote._tcp.local.")}},
		[]record{{name: "h.local.", rtype: typeA, flush: true, ttl: 120, data: aData(net.IPv4(192, 168, 1, 5))}})
	if binary.BigEndian.Uint16(resp[2:]) != 0x8400 || binary.BigEndian.Uint16(resp[6:]) != 1 || binary.BigEndian.Uint16(resp[10:]) != 1 {
		t.Fatalf("bad header %x", resp[:12])
	}
	if r, _ := parseQuery(resp); r != nil {
		t.Error("responses must not be treated as queries")
	}
	// Compressed name: pointer back to offset 12.
	c := append(append([]byte{}, q...), 0xC0, 12)
	name, _, err := readName(c, len(q))
	if err != nil || name != "_korokmote._tcp.local." {
		t.Errorf("compressed name: %q %v", name, err)
	}
}

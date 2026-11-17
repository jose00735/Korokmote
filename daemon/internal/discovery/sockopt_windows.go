//go:build windows

package discovery

import (
	"net"
	"syscall"
	"unsafe"
)

const (
	ipMulticastIF  = 9
	ipMulticastTTL = 10
	ipAddMember    = 12
)

func setMulticastIface(c *net.UDPConn, ip net.IP) error {
	raw, err := c.SyscallConn()
	if err != nil {
		return err
	}
	addr := [4]byte(ip.To4())
	var serr error
	err = raw.Control(func(fd uintptr) {
		serr = syscall.Setsockopt(syscall.Handle(fd), syscall.IPPROTO_IP, ipMulticastIF, &addr[0], 4)
		if serr == nil {
			ttl := int32(255)
			serr = syscall.Setsockopt(syscall.Handle(fd), syscall.IPPROTO_IP, ipMulticastTTL, (*byte)(unsafe.Pointer(&ttl)), 4)
		}
	})
	if err != nil {
		return err
	}
	return serr
}

func joinGroup(c *net.UDPConn, group, local net.IP) error {
	raw, err := c.SyscallConn()
	if err != nil {
		return err
	}
	var mreq [8]byte
	copy(mreq[0:4], group.To4())
	copy(mreq[4:8], local.To4())
	var serr error
	err = raw.Control(func(fd uintptr) {
		serr = syscall.Setsockopt(syscall.Handle(fd), syscall.IPPROTO_IP, ipAddMember, &mreq[0], 8)
	})
	if err != nil {
		return err
	}
	return serr
}

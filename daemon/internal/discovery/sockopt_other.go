//go:build !windows

package discovery

import (
	"net"
	"syscall"
)

func setMulticastIface(c *net.UDPConn, ip net.IP) error {
	raw, err := c.SyscallConn()
	if err != nil {
		return err
	}
	var serr error
	err = raw.Control(func(fd uintptr) {
		serr = syscall.SetsockoptInet4Addr(int(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_IF, [4]byte(ip.To4()))
		if serr == nil {
			serr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, 255)
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
	mreq := &syscall.IPMreq{Multiaddr: [4]byte(group.To4()), Interface: [4]byte(local.To4())}
	var serr error
	err = raw.Control(func(fd uintptr) {
		serr = syscall.SetsockoptIPMreq(int(fd), syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreq)
	})
	if err != nil {
		return err
	}
	return serr
}

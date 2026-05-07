//go:build windows

package netutil

import (
	"context"
	"net"
	"syscall"
)

// ListenUDP6Only binds an IPv6 UDP socket with IPV6_V6ONLY set, so a
// separate IPv4 socket can bind the same port and receive IPv4 broadcasts
// (255.255.255.255), which a dual-stack IPv6 socket does not receive.
func ListenUDP6Only(address string) (*net.UDPConn, error) {
	lc := net.ListenConfig{
		Control: func(network, addr string, c syscall.RawConn) error {
			var sockErr error
			if err := c.Control(func(fd uintptr) {
				sockErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, 1)
			}); err != nil {
				return err
			}
			return sockErr
		},
	}
	pc, err := lc.ListenPacket(context.Background(), "udp6", address)
	if err != nil {
		return nil, err
	}
	return pc.(*net.UDPConn), nil
}

package client

import (
	"net"

	"golang.org/x/sys/unix"
)

func enableDSCP(fd int, localAddr net.IP, dscp int) error {
	if localAddr.To4() == nil {
		if err := unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_TCLASS, dscp<<2); err != nil {
			return err
		}
	} else {
		if err := unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_TOS, dscp<<2); err != nil {
			return err
		}
	}
	return nil
}

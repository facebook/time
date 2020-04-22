/*
Copyright (c) Facebook, Inc. and its affiliates.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ntp

import (
	"fmt"
	"net"

	syscall "golang.org/x/sys/unix"
)

// EnableKernelTimestampsSocket enables socket options to read ether kernel timestamps
func EnableKernelTimestampsSocket(conn *net.UDPConn) error {
	// Get socket fd
	connfd, err := connFd(conn)
	if err != nil {
		return err
	}

	// Allow reading of hardware timestamps via socket
	if err := syscall.SetsockoptInt(connfd, syscall.SOL_SOCKET, syscall.SO_TIMESTAMP, 1); err != nil {
		return fmt.Errorf("failed to enable SO_TIMESTAMP: %w", err)
	}
	return nil
}

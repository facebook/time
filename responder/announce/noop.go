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

/*
Package announce implements announcement of server IPs.
Depending on the implementation it could be anything:
* Exabgp
* Haproxy
* Carp
*/
package announce

import (
	"net"
)

// NoopAnnounce is a noop implementation of Announce interface
// Use it if no IP advertisement is required
type NoopAnnounce struct{}

// Advertise is implementing Advertise interface. Doing nothing
func (n *NoopAnnounce) Advertise([]net.IP) error {
	return nil
}

// Withdraw is implementing Withdraw interface. Doing nothing
func (n *NoopAnnounce) Withdraw() error {
	return nil
}

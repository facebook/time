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
Package control implements NTP Control Protocol (RFC1305, Appendix B).

To make it more useful, some implementation details diverge from RFC to match what is
described in NTPD docs http://doc.ntp.org/current-stable/decode.html.

Library allows communicating with any NTP server that implements Control Protocol (such as ntpd),
and get various information, for example: current server status; server variables like offset; peers with their statuses and variables; server counters.

Example usage can be found in ntpcheck project - https://github.com/facebookincubator/time/ntp/ntpcheck
*/
package control

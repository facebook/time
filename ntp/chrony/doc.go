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
Package chrony implements Chrony (https://chrony.tuxfamily.org) network protocol v6 used for monitoring of the timeserver.

As of now, only monitoring part of protocol that is used to communicate between `chronyc` and `chronyd` is implemented.
Chronyc/chronyd protocol is not documented (https://chrony.tuxfamily.org/faq.html#_is_the_code_chronyc_code_code_chronyd_code_protocol_documented_anywhere).

Library allows communicating with Chrony NTP server,
and get various information, for example: current server status; server variables like offset; peers with their statuses and variables; server counters.

Example usage can be found in ntpcheck project - https://github.com/facebook/time/ntp/ntpcheck
*/
package chrony

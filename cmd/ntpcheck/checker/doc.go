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
Package checker provides an easy way to talk to NTPD/Chronyd
and get NTPCheckResult, which abstracts away Chrony/NTP control protocol implementation details
and contains tons of information useful for NTP monitoring,
like NTP server variables including offset, peers and their variables and statuses.
*/
package checker

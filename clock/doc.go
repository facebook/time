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
Package clock contains a wrapper around CLOCK_ADJTIME syscall.

It allows interactions with supported clocks, such as system realtime clock or PHC.

Supported methods include 
 - calling CLOCK_ADJTIME syscall through Adjtime 
 - getting the frequency through FrequencyPPB, which reads and converts the clock's current
   frequency to PPB.
 - adjusting the frequency through AdjFreqPPB 
 - stepping the clock through Step function, which adjusts the clock forwards or backwards
   by a given step size.
 - returning maximum frequency adjustment possible for the clock
 - updating clock's status after synchronization.

The purpose of this library is to allow access to system clocks and allow for precise
adjustments through stepping forwards and backwards. 
 - grants ability for precise timekeeping in systems, which is essential to NTP
 - allows for proper synchronization between networks
 - creates accurate time-series data for storage in databases

*/
package clock

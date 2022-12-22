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

#pragma once

#include <linux/ptp_clock.h>

// as in
// https://github.com/torvalds/linux/blob/master/include/uapi/linux/ptp_clock.h
#ifndef PTP_SYS_OFFSET_EXTENDED

#define PTP_SYS_OFFSET_EXTENDED \
  _IOWR(PTP_CLK_MAGIC, 9, struct ptp_sys_offset_extended)

struct ptp_sys_offset_extended {
  unsigned int n_samples; /* Desired number of measurements. */
  unsigned int rsv[3]; /* Reserved for future use. */
  /*
   * Array of [system, phc, system] time stamps. The kernel will provide
   * 3*n_samples time stamps.
   */
  struct ptp_clock_time ts[PTP_MAX_SAMPLES][3];
};

#endif

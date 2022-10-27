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

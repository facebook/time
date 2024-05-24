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

#include "fbclock.h"
#include <fcntl.h> // For O_* constants
#include <linux/ptp_clock.h>
#include <math.h> // pow
#include <stdint.h>
#include <stdio.h> // for printf and perror
#include <string.h>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h> // close
#include "missing.h"

#ifndef NDEBUG
#define fbclock_debug_print(fmt, ...)  \
  do {                                 \
    fprintf(stderr, fmt, __VA_ARGS__); \
  } while (0)
#else
#define fbclock_debug_print(fmt, ...)
#endif

#define FBCLOCK_CLOCKDATA_SIZE sizeof(fbclock_clockdata)
#define FBCLOCK_MAX_READ_TRIES 1000
#define NANOSECONDS_IN_SECONDS 1e9

#ifdef __x86_64__
#define fbclock_crc64 __builtin_ia32_crc32di
#endif

#ifdef __aarch64__
#ifdef __SSE4_2__
#define fbclock_crc64 _mm_crc32_u64
#endif
#endif

// dumb replacement for platforms we don't fully support
#ifndef fbclock_crc64
#define fbclock_crc64(a, b) ({ a ^ b; })
#endif

struct phc_time_res {
  int64_t ts; // last ts got from PHC
  int64_t delay; // mean delay of several requests
};

static inline uint64_t fbclock_clockdata_crc_DEPRECATED(
    fbclock_clockdata* value) {
  // TODO: Remove deprecated function after phase is rolled out.
  uint64_t counter = fbclock_crc64(value->ingress_time_ns, 0x04C11DB7);
  counter = fbclock_crc64(value->error_bound_ns, counter);
  counter = fbclock_crc64(value->holdover_multiplier_ns, counter);
  return counter;
}

static inline uint64_t fbclock_clockdata_crc(fbclock_clockdata* value) {
  uint64_t counter = fbclock_crc64(0xFFFFFFFF, value->ingress_time_ns);
  counter = fbclock_crc64(counter, value->error_bound_ns);
  counter = fbclock_crc64(counter, value->holdover_multiplier_ns);
  return counter ^ 0xFFFFFFFF;
}

int fbclock_clockdata_store_data(uint32_t fd, fbclock_clockdata* data) {
  fbclock_shmdata* shmp = mmap(
      NULL, FBCLOCK_SHMDATA_SIZE, PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0);
  if (shmp == MAP_FAILED) {
    return FBCLOCK_E_SHMEM_MAP_FAILED;
  }
  uint64_t crc = fbclock_clockdata_crc(data);
  memcpy(&shmp->data, data, FBCLOCK_CLOCKDATA_SIZE);
  atomic_store(&shmp->crc, crc);
  munmap(shmp, FBCLOCK_SHMDATA_SIZE);
  return FBCLOCK_E_NO_ERROR;
}

int fbclock_clockdata_load_data(
    fbclock_shmdata* shmp,
    fbclock_clockdata* data) {
  for (int i = 0; i < FBCLOCK_MAX_READ_TRIES; i++) {
    memcpy(data, &shmp->data, FBCLOCK_CLOCKDATA_SIZE);
    uint64_t crc = atomic_load(&shmp->crc);
    uint64_t our_crc = fbclock_clockdata_crc(data);
    if (our_crc == crc) {
      fbclock_debug_print("reading clock data took %d tries\n", i + 1);
      return FBCLOCK_E_NO_ERROR;
    }
    // TODO: Remove deprecated block after phase is rolled out.
    uint64_t our_crc_DEPRECATED = fbclock_clockdata_crc_DEPRECATED(data);
    if (our_crc_DEPRECATED == crc) {
      fbclock_debug_print("reading clock data took %d tries (old)\n", i + 1);
      return FBCLOCK_E_NO_ERROR;
    }
  }
  fbclock_debug_print(
      "failed to read clock data after %d tries\n", FBCLOCK_MAX_READ_TRIES);
  // TODO: Enable mismatch error.
  // return FBCLOCK_E_CRC_MISMATCH;
  return FBCLOCK_E_NO_ERROR;
}

static inline int64_t fbclock_pct2ns(const struct ptp_clock_time* ptc) {
  return (int64_t)(ptc->sec * NANOSECONDS_IN_SECONDS) + (int64_t)ptc->nsec;
}

static int fbclock_read_ptp_offset(int fd, struct phc_time_res* res) {
  struct ptp_sys_offset pso = {.n_samples = 5};
  int64_t min_delay = INT64_MAX, last_ts;

  int r = ioctl(fd, PTP_SYS_OFFSET, &pso);
  if (r) {
    perror("PTP_SYS_OFFSET");
    return -1;
  }

  for (unsigned i = 0; i < pso.n_samples; ++i) {
    int64_t delay =
        fbclock_pct2ns(&pso.ts[2 * i + 2]) - fbclock_pct2ns(&pso.ts[2 * i]);
    min_delay = (delay < min_delay) ? delay : min_delay;
    last_ts = fbclock_pct2ns(&pso.ts[2 * i + 1]);
  }
  res->ts = last_ts;
  res->delay = min_delay;
  if (min_delay < 0) {
    perror("Negative request delay");
    return -2;
  }
  return 0;
}

static int fbclock_read_ptp_offset_extended(int fd, struct phc_time_res* res) {
  struct ptp_sys_offset_extended psoe = {.n_samples = 5};
  int64_t min_delay = INT64_MAX;

  int r = ioctl(fd, PTP_SYS_OFFSET_EXTENDED, &psoe);
  if (r) {
    perror("PTP_SYS_OFFSET_EXTENDED");
    return -1;
  }

  for (unsigned i = 0; i < psoe.n_samples; ++i) {
    int64_t delay =
        fbclock_pct2ns(&psoe.ts[i][2]) - fbclock_pct2ns(&psoe.ts[i][0]);
    min_delay = (delay < min_delay) ? delay : min_delay;
  }
  res->ts = fbclock_pct2ns(&psoe.ts[psoe.n_samples - 1][1]);
  res->delay = min_delay;
  if (min_delay < 0) {
    perror("Negative request delay");
    return -2;
  }
  return 0;
}

int fbclock_init(fbclock_lib* lib, const char* shm_path) {
  lib->ptp_path = FBCLOCK_PTPPATH;
  int sfd = open(shm_path, O_RDONLY, 0);
  if (sfd == -1) {
    perror("open shmem device");
    return FBCLOCK_E_SHMEM_OPEN;
  }
  lib->shm_fd = sfd;

  int ffd = open(lib->ptp_path, O_RDONLY);
  if (ffd == -1) {
    perror("open PTP device");
    return FBCLOCK_E_PTP_OPEN;
  }
  lib->dev_fd = ffd;

  struct ptp_sys_offset_extended psoe = {.n_samples = 1};

  int r = ioctl(ffd, PTP_SYS_OFFSET_EXTENDED, &psoe);
  if (!r) {
    lib->gettime = fbclock_read_ptp_offset_extended;
  } else {
    lib->gettime = fbclock_read_ptp_offset;
  }

  fbclock_shmdata* shmp =
      mmap(NULL, FBCLOCK_SHMDATA_SIZE, PROT_READ, MAP_SHARED, lib->shm_fd, 0);
  if (shmp == MAP_FAILED) {
    return FBCLOCK_E_SHMEM_MAP_FAILED;
  }
  lib->shmp = shmp;
  return FBCLOCK_E_NO_ERROR;
}

int fbclock_destroy(fbclock_lib* lib) {
  munmap(lib->shmp, FBCLOCK_SHMDATA_SIZE);
  close(lib->dev_fd);
  close(lib->shm_fd);
  return FBCLOCK_E_NO_ERROR;
  // we don't want to unlink it, others might still use it
}

double fbclock_window_of_uncertainty(
    double seconds,
    double error_bound_ns,
    double holdover_multiplier_ns) {
  double h = holdover_multiplier_ns * seconds;
  double w = error_bound_ns + h;
  fbclock_debug_print("error_bound=%f\n", error_bound_ns);
  fbclock_debug_print("holdover_multiplier=%f\n", holdover_multiplier_ns);
  fbclock_debug_print("%.3f seconds holdover, h=%f\n", seconds, h);
  fbclock_debug_print("w = %f ns\n", w);
  fbclock_debug_print("w = %f ms\n", w / 1000000.0);
  return w;
}

int fbclock_calculate_time(
    double error_bound_ns,
    double h_value_ns,
    fbclock_clockdata* state,
    int64_t phctime_ns,
    fbclock_truetime* truetime,
    int time_standard) {
  // check how far back since last SYNC message from GM (in seconds)
  double seconds =
      (double)(phctime_ns - state->ingress_time_ns) / NANOSECONDS_IN_SECONDS;

  if (seconds < 0) {
    return FBCLOCK_E_PHC_IN_THE_PAST;
  }

  // UTC offset applied if time standard used is UTC (and not TAI)
  if (time_standard == FBCLOCK_UTC) {
    phctime_ns = fbclock_apply_utc_offset(state, phctime_ns);
  }

  // calculate the Window of Uncertainty (WOU) (in nanoseconds)
  double wou_ns =
      fbclock_window_of_uncertainty(seconds, error_bound_ns, h_value_ns);
  truetime->earliest_ns = phctime_ns - (uint64_t)wou_ns;
  truetime->latest_ns = phctime_ns + (uint64_t)wou_ns;
  return FBCLOCK_E_NO_ERROR;
}

int fbclock_gettime_tz(
    fbclock_lib* lib,
    fbclock_truetime* truetime,
    int timezone) {
  struct phc_time_res res;
  fbclock_clockdata state = {};
  int rcode = fbclock_clockdata_load_data(lib->shmp, &state);
  if (rcode != FBCLOCK_E_NO_ERROR) {
    return rcode;
  }

  // cannot determine Truetime without these values
  if (state.error_bound_ns == 0 || state.ingress_time_ns == 0) {
    return FBCLOCK_E_NO_DATA;
  }

  // if the value is stored as UINT32_MAX then it's too big
  if (state.error_bound_ns == UINT32_MAX ||
      state.holdover_multiplier_ns == UINT32_MAX) {
    return FBCLOCK_E_WOU_TOO_BIG;
  }

  if (lib->gettime(lib->dev_fd, &res)) {
    return FBCLOCK_E_PTP_READ_OFFSET;
  }

  double error_bound = (double)state.error_bound_ns + (double)res.delay;
  double h_value = (double)state.holdover_multiplier_ns / FBCLOCK_POW2_16;

  return fbclock_calculate_time(
      error_bound, h_value, &state, res.ts, truetime, timezone);
}

int fbclock_gettime(fbclock_lib* lib, fbclock_truetime* truetime) {
  return fbclock_gettime_tz(lib, truetime, FBCLOCK_TAI);
}

int fbclock_gettime_utc(fbclock_lib* lib, fbclock_truetime* truetime) {
  return fbclock_gettime_tz(lib, truetime, FBCLOCK_UTC);
}

uint64_t fbclock_apply_smear(
    uint64_t time,
    uint64_t offset_pre_ns,
    uint64_t offset_post_ns,
    uint64_t smear_start_ns,
    uint64_t smear_end_ns,
    int multiplier) {
  if (time > smear_end_ns) {
    time -= offset_post_ns;
  } else if (time < smear_start_ns) {
    time -= offset_pre_ns;
  } else if (smear_start_ns <= time && time <= smear_end_ns) {
    uint64_t smear = multiplier * ((time - smear_start_ns) / SMEAR_STEP_NS);
    time -= (offset_pre_ns + smear);
  }
  return time;
}

uint64_t fbclock_apply_utc_offset(
    fbclock_clockdata* state,
    int64_t phctime_ns) {
  // Fixed offset is applied if tzdata information not in shared memory
  if (state->utc_offset_pre_s == 0 && state->utc_offset_post_s == 0) {
    phctime_ns += UTC_TAI_OFFSET_NS;
    return (uint64_t)phctime_ns;
  }

  fbclock_debug_print(
      "UTC-TAI Offset Before Leap Second Event: %d\n", state->utc_offset_pre_s);
  fbclock_debug_print(
      "UTC-TAI Offset After Leap Second Event: %d\n", state->utc_offset_post_s);
  fbclock_debug_print(
      "Clock Smearing Start Time (TAI): %lu\n", state->clock_smearing_start_s);
  fbclock_debug_print(
      "Clock Smearing End Time (TAI): %lu\n", state->clock_smearing_end_s);

  // Multipler may be negative (if a negative leap second is applied)
  int multiplier = state->utc_offset_post_s - state->utc_offset_pre_s;

  // Switch to nanoseconds
  uint64_t smear_end_ns = state->clock_smearing_end_s * 1e9;
  uint64_t smear_start_ns = state->clock_smearing_start_s * 1e9;
  uint64_t offset_post_ns = state->utc_offset_post_s * 1e9;
  uint64_t offset_pre_ns = state->utc_offset_pre_s * 1e9;

  return fbclock_apply_smear(
      phctime_ns,
      offset_pre_ns,
      offset_post_ns,
      smear_start_ns,
      smear_end_ns,
      multiplier);
}

const char* fbclock_strerror(int err_code) {
  const char* err_info = "unknown error";
  switch (err_code) {
    case FBCLOCK_E_SHMEM_MAP_FAILED:
      err_info = "shmem map error";
      break;
    case FBCLOCK_E_SHMEM_OPEN:
      err_info = "shmem open error";
      break;
    case FBCLOCK_E_PTP_READ_OFFSET:
      err_info = "PTP PTP_SYS_OFFSET_EXTENDED ioctl error";
      break;
    case FBCLOCK_E_PTP_OPEN:
      err_info = "PTP device open error";
      break;
    case FBCLOCK_E_NO_DATA:
      err_info = "no data from daemon error";
      break;
    case FBCLOCK_E_WOU_TOO_BIG:
      err_info = "WOU is too big";
      break;
    case FBCLOCK_E_PHC_IN_THE_PAST:
      err_info = "PHC jumped back in time";
      break;
    case FBCLOCK_E_CRC_MISMATCH:
      err_info = "CRC check failed all tries";
      break;
    case FBCLOCK_E_NO_ERROR:
      err_info = "no error";
      break;
    default:
      err_info = "unknown error";
      break;
  }
  return err_info;
}

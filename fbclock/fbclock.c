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

static uint64_t fbclock_clockdata_crc(fbclock_clockdata* value) {
  uint64_t counter = fbclock_crc64(value->ingress_time_ns, 0x04C11DB7);
  counter = fbclock_crc64(value->error_bound_ns, counter);
  counter = fbclock_crc64(value->holdover_multiplier_ns, counter);
  return counter;
}

// fbclock_clockdata_store_data is used in shmem.go to store timing data
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
  return 0;
}

int fbclock_clockdata_load_data(
    fbclock_shmdata* shmp,
    fbclock_clockdata* data) {
  for (int i = 0; i < FBCLOCK_MAX_READ_TRIES; i++) {
    memcpy(data, &shmp->data, FBCLOCK_CLOCKDATA_SIZE);
    uint64_t our_crc = fbclock_clockdata_crc(data);
    uint64_t crc = atomic_load(&shmp->crc);
    if (our_crc == crc) {
      fbclock_debug_print("reading clock data took %d tries\n", i + 1);
      break;
    }
  }
  return 0;
}

static int64_t fbclock_pct2ns(const struct ptp_clock_time* ptc) {
  return (int64_t)(ptc->sec * 1000000000) + (int64_t)ptc->nsec;
}

static int fbclock_read_ptp_offset_extended(int fd, struct phc_time_res* res) {
  struct ptp_sys_offset_extended psoe = {.n_samples = 5};
  int64_t total_delay = 0;

  int r = ioctl(fd, PTP_SYS_OFFSET_EXTENDED, &psoe);
  if (r) {
    perror("PTP_SYS_OFFSET_EXTENDED");
    return -1;
  }

  for (unsigned i = 0; i < psoe.n_samples; ++i) {
    total_delay +=
        fbclock_pct2ns(&psoe.ts[i][2]) - fbclock_pct2ns(&psoe.ts[i][0]);
  }
  res->ts = fbclock_pct2ns(&psoe.ts[psoe.n_samples - 1][1]);
  res->delay = total_delay / psoe.n_samples; // mean delay
  if (total_delay < 0) {
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

  fbclock_shmdata* shmp =
      mmap(NULL, FBCLOCK_SHMDATA_SIZE, PROT_READ, MAP_SHARED, lib->shm_fd, 0);
  if (shmp == MAP_FAILED) {
    return FBCLOCK_E_SHMEM_MAP_FAILED;
  }
  lib->shmp = shmp;
  return 0;
}

int fbclock_destroy(fbclock_lib* lib) {
  munmap(lib->shmp, FBCLOCK_SHMDATA_SIZE);
  close(lib->dev_fd);
  close(lib->shm_fd);
  return 0;
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
    int64_t ingress_time_ns,
    int64_t phctime_ns,
    fbclock_truetime* truetime) {
  // first, we check how long it was since last SYNC message from GM, in seconds
  // with parts
  double seconds = (double)(phctime_ns - ingress_time_ns) / 1000000000.0;
  if (seconds < 0) {
    return FBCLOCK_E_PHC_IN_THE_PAST;
  }
  // then we calculate WOU
  double wou_ns =
      fbclock_window_of_uncertainty(seconds, error_bound_ns, h_value_ns);
  truetime->earliest_ns = phctime_ns - (uint64_t)wou_ns;
  truetime->latest_ns = phctime_ns + (uint64_t)wou_ns;
  return 0;
}

int fbclock_gettime(fbclock_lib* lib, fbclock_truetime* truetime) {
  struct phc_time_res res;
  fbclock_clockdata state;
  int rcode = fbclock_clockdata_load_data(lib->shmp, &state);
  if (rcode != 0) {
    return rcode;
  }

  // if by this point we still haven't managed to get consistent data - go ahead
  // with potential inconsistency
  if (state.error_bound_ns == 0 || state.ingress_time_ns == 0) {
    return FBCLOCK_E_NO_DATA;
  }

  // if the value is stored as UINT32_MAX then it's too big
  if (state.error_bound_ns == UINT32_MAX ||
      state.holdover_multiplier_ns == UINT32_MAX) {
    return FBCLOCK_E_WOU_TOO_BIG;
  }

  if (fbclock_read_ptp_offset_extended(lib->dev_fd, &res)) {
    return FBCLOCK_E_PTP_READ_OFFSET;
  }

  double error_bound = (double)state.error_bound_ns + (double)res.delay;
  double h_value = (double)state.holdover_multiplier_ns / FBCLOCK_POW2_16;
  return fbclock_calculate_time(
      error_bound, h_value, state.ingress_time_ns, res.ts, truetime);
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
    case 0:
      err_info = "no error";
      break;
    default:
      err_info = "unknown error";
      break;
  }
  return err_info;
}

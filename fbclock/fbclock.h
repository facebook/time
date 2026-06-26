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

#if defined(__cplusplus) && !defined(__clang__)
#include <atomic>
typedef std::atomic_uint_fast64_t atomic_uint64;
#else
#include <stdatomic.h>
typedef atomic_uint_fast64_t atomic_uint64;
#endif

#include <stdint.h> /* for proper fixed width types */
#ifndef __cplusplus
#include <stdalign.h> /* for alignas in C; alignas is a keyword in C++ */
#endif

// _Nonnull is a Clang-only nullability qualifier. The open-source build
// compiles this header with gcc (via cgo and the fbclock-bin RPM), which
// doesn't know it, so make it a no-op there. Clang keeps the real keyword for
// nullsafeclang.
#if !defined(__clang__)
#define _Nonnull
#endif

// error codes
#define FBCLOCK_E_NO_ERROR 0
#define FBCLOCK_E_SHMEM_MAP_FAILED -1
#define FBCLOCK_E_SHMEM_OPEN -2
#define FBCLOCK_E_PTP_READ_OFFSET -3
#define FBCLOCK_E_PTP_OPEN -4
#define FBCLOCK_E_NO_DATA -5
#define FBCLOCK_E_WOU_TOO_BIG -6
#define FBCLOCK_E_PHC_IN_THE_PAST -7
#define FBCLOCK_E_CRC_MISMATCH -8
#define FBCLOCK_E_DATA_STALE -9

// Fixed UTC-TAI offset - used when data not present in shared memory
#define UTC_TAI_OFFSET_NS (int64_t)(-37e9)
// Smear step size - smear clock by 1ns every 65us
#define SMEAR_STEP_NS (int64_t)(65e3)
// Maximum gap (ns) between cached daemon sysclock sample and current sysclock
// before the V2 client refuses to extrapolate. Daemon updates every 10 ms; this
// is a 100x failsafe — if the daemon is dead or the snapshot is corrupt,
// extrapolation error is unbounded so we'd rather return an error.
#define FBCLOCK_MAX_EXTRAPOLATION_NS (int64_t)(1000000000)

#ifdef __cplusplus
extern "C" {
#endif

struct phc_time_res;

typedef struct fbclock_clockdata {
  // PHC time when ptp client last time received sync message
  int64_t ingress_time_ns;
  // error bound calculated based on PTP client GM offset, path delay and
  // frequency adjustment
  uint32_t error_bound_ns;
  // multiplier we use to adjust error bound when clock in holdover mode
  uint32_t holdover_multiplier_ns;
  // start time (TAI) to begin smearing clock
  uint64_t clock_smearing_start_s;
  // end time (TAI) to stop smearing clock
  uint64_t clock_smearing_end_s;
  // UTC offset before latest published leap second (tzdata)
  int32_t utc_offset_pre_s;
  // UTC offset after latest published leap second (tzdata)
  int32_t utc_offset_post_s;

} fbclock_clockdata;

typedef struct fbclock_clockdata_v2 {
  // PHC time when ptp client last time received sync message
  int64_t ingress_time_ns;
  // error bound calculated based on PTP client GM offset, path delay and
  // frequency adjustment
  uint32_t error_bound_ns;
  // multiplier we use to adjust error bound when clock in holdover mode
  uint32_t holdover_multiplier_ns;
  // start time (TAI) to begin smearing clock
  uint64_t clock_smearing_start_s;
  // UTC offset before latest published leap second (tzdata)
  int16_t utc_offset_pre_s;
  // UTC offset after latest published leap second (tzdata)
  int16_t utc_offset_post_s;
  // we may have sys clock read with MONOTONIC_RAW or REALTIME clock source
  uint32_t clockId;
  // periodically updated PHC time
  int64_t phc_time_ns;
  // system clock time received during periodical update of PHC time
  int64_t sysclock_time_ns;
  // extrapolation coefficient in PPB
  int64_t coef_ppb;
} fbclock_clockdata_v2;

// fbclock shared memory object
typedef struct fbclock_shmdata {
  atomic_uint64 crc;
  fbclock_clockdata data;
} fbclock_shmdata;

// One seqlock-protected anchor section. The v2 region holds two of these on
// separate cache lines (forced by alignas) so a write to one never invalidates
// the other reader's cache line or bumps the other's counter.
typedef struct fbclock_shmsection {
  alignas(64) atomic_uint64 seq;
  fbclock_clockdata_v2 data;
} fbclock_shmsection;

// fbclock v2 shared memory object: two homogeneous anchor sections, each a
// seqlock-protected fbclock_clockdata_v2 (see fbclock_shmsection).
//  - primary: the anchor fbclock_gettime reads. MONOTONIC_RAW, or REALTIME on
//    hosts whose kernel rejects MONOTONIC_RAW in PTP_SYS_OFFSET_EXTENDED.
//  - realtime: a dedicated REALTIME-domain anchor for fbclock_gettime_past,
//    written by the daemon only when the primary is MONOTONIC_RAW. When the
//    primary is already REALTIME it is itself the realtime anchor, so
//    gettime_past uses it directly and this section stays empty.
// Each section carries its own clockId; gettime_past selects by clock domain.
typedef struct fbclock_shmdata_v2 {
  fbclock_shmsection primary;
  fbclock_shmsection realtime;
} fbclock_shmdata_v2;

#define FBCLOCK_SHMDATA_SIZE sizeof(fbclock_shmdata)
#define FBCLOCK_SHMDATA_V2_SIZE sizeof(fbclock_shmdata_v2)
#define FBCLOCK_PATH_V1 "/run/fbclock_data_v1"
#define FBCLOCK_PATH "/run/fbclock_data_v2"
#define FBCLOCK_POW2_16 ((double)(1ULL << 16))
#define FBCLOCK_PTPPATH "/dev/fbclock/ptp"

// supported time standards
#define FBCLOCK_TAI 0
#define FBCLOCK_UTC 1

// response to fbclock_gettime request
typedef struct fbclock_truetime {
  uint64_t earliest_ns;
  uint64_t latest_ns;
} fbclock_truetime;

// fbclock library
typedef struct fbclock_lib {
  char* ptp_path; // path to PHC clock device
  int shm_fd; // file descriptor of opened shared memory object
  int dev_fd; // file descriptor of opened /dev/ptpN
  int64_t min_phc_delay; // minimal PHC request delay observed
  fbclock_shmdata* shmp; // mmap-ed data
  fbclock_shmdata_v2* shmp_v2; // mmap-ed data
  int (*gettime)(int, struct phc_time_res*); // pointer to gettime function
} fbclock_lib;

int fbclock_clockdata_store_data(uint32_t fd, fbclock_clockdata* data);
// Writes the primary anchor section.
int fbclock_clockdata_store_data_v2(uint32_t fd, fbclock_clockdata_v2* data);
// Writes the realtime anchor section, used on hosts whose primary is
// MONOTONIC_RAW. The daemon writes it separately from (and after) the primary
// section; when the primary is already REALTIME it is skipped.
int fbclock_clockdata_store_data_realtime(
    uint32_t fd,
    fbclock_clockdata_v2* data);
int fbclock_clockdata_load_data(fbclock_shmdata* shm, fbclock_clockdata* data);
int fbclock_clockdata_load_data_v2(
    fbclock_shmdata_v2* shmp,
    fbclock_clockdata_v2* data);
int fbclock_clockdata_load_data_realtime(
    fbclock_shmdata_v2* shmp,
    fbclock_clockdata_v2* data);
uint64_t fbclock_window_of_uncertainty(
    double seconds,
    uint64_t error_bound_ns,
    double holdover_multiplier_ns);
int fbclock_calculate_time(
    uint64_t error_bound_ns,
    double h_value_ns,
    fbclock_clockdata* state,
    int64_t phctime_ns,
    fbclock_truetime* truetime,
    int timezone);
int fbclock_calculate_time_v2(
    uint64_t error_bound_ns,
    double h_value_ns,
    fbclock_clockdata_v2* state,
    int64_t sysclock_time_now_ns,
    fbclock_truetime* truetime,
    int timezone);
int fbclock_calculate_time_past_v2(
    uint64_t error_bound_ns,
    double h_value_ns,
    fbclock_clockdata_v2* _Nonnull state,
    int64_t ts_realtime_ns,
    fbclock_truetime* _Nonnull truetime,
    int timezone);
uint64_t fbclock_apply_utc_offset(fbclock_clockdata* state, int64_t phctime_ns);
uint64_t fbclock_apply_utc_offset_v2(
    fbclock_clockdata_v2* state,
    int64_t phctime_ns);
uint64_t fbclock_apply_smear(
    uint64_t time,
    uint64_t offset_pre_ns,
    uint64_t offset_post_ns,
    uint64_t smear_start_ns,
    uint64_t smear_end_ns,
    int multiplier);
int fbclock_gettime_tz(
    fbclock_lib* lib,
    fbclock_truetime* truetime,
    int timezone);

// methods we provide to end users
int fbclock_init(fbclock_lib* lib, const char* shm_path);
int fbclock_destroy(fbclock_lib* lib);
int fbclock_gettime(fbclock_lib* lib, fbclock_truetime* truetime);
int fbclock_gettime_utc(fbclock_lib* lib, fbclock_truetime* truetime);
// fbclock_gettime_past translates ts (CLOCK_REALTIME ns since epoch) into a
// TAI [earliest, latest] window in PHC domain. ts must be from the same host
// the lib is running on, within FBCLOCK_MAX_EXTRAPOLATION_NS of the daemon's
// most recent REALTIME snapshot (in either direction). v2-only.
int fbclock_gettime_past(
    fbclock_lib* _Nonnull lib,
    int64_t ts_realtime_ns,
    fbclock_truetime* _Nonnull truetime);
int fbclock_gettime_past_utc(
    fbclock_lib* _Nonnull lib,
    int64_t ts_realtime_ns,
    fbclock_truetime* _Nonnull truetime);

// turn error code into err msg
const char* fbclock_strerror(int err_code);

#ifdef __cplusplus
} // extern "C"
#endif

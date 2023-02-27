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

// error codes
#define FBCLOCK_E_SHMEM_MAP_FAILED -1
#define FBCLOCK_E_SHMEM_OPEN -2
#define FBCLOCK_E_PTP_READ_OFFSET -3
#define FBCLOCK_E_PTP_OPEN -4
#define FBCLOCK_E_NO_DATA -5
#define FBCLOCK_E_WOU_TOO_BIG -6
#define FBCLOCK_E_PHC_IN_THE_PAST -7

#ifdef __cplusplus
extern "C" {
#endif

typedef struct fbclock_clockdata {
  int64_t ingress_time_ns; // PHC time when ptp client last time received sync
                           // message
  uint32_t error_bound_ns; // error bound as calculated based on PTP client GM
                           // offset, path delay and frequency adjustment
  uint32_t holdover_multiplier_ns; // multiplier we use to adjust error bound
                                   // when clock is running in holdover mode
} fbclock_clockdata;

// Define a structure that will be imposed on the shared memory object.
typedef struct fbclock_shmdata {
  atomic_uint64 crc;
  fbclock_clockdata data;
} fbclock_shmdata;

#define FBCLOCK_SHMDATA_SIZE sizeof(fbclock_shmdata)
#define FBCLOCK_PATH "/dev/shm/fbclock_data_v1"
#define FBCLOCK_POW2_16 ((double)(1ULL << 16))
#define FBCLOCK_PTPPATH "/dev/fbclock/ptp"

// library
typedef struct fbclock_lib {
  char* ptp_path; // path to PHC clock device
  int shm_fd; // file descriptor of opened shared mem
  int dev_fd; // file descriptor of opened /dev/ptpN
  fbclock_shmdata* shmp; // mmap-ed data
} fbclock_lib;

// what customers get
typedef struct fbclock_truetime {
  uint64_t earliest_ns;
  uint64_t latest_ns;
} fbclock_truetime;

int fbclock_clockdata_store_data(uint32_t fd, fbclock_clockdata* data);
int fbclock_clockdata_load_data(fbclock_shmdata* shm, fbclock_clockdata* data);
double fbclock_window_of_uncertainty(
    double seconds,
    double error_bound_ns,
    double holdover_multiplier_ns);
int fbclock_calculate_time(
    double error_bound_ns,
    double h_value_ns,
    int64_t ingress_time_ns,
    int64_t phctime_ns,
    fbclock_truetime* truetime);

// methods we provide to end users
int fbclock_init(fbclock_lib* lib, const char* shm_path);
int fbclock_destroy(fbclock_lib* lib);
int fbclock_gettime(fbclock_lib* lib, fbclock_truetime* truetime);
// turn error code into err msg
const char* fbclock_strerror(int err_code);

#ifdef __cplusplus
} // extern "C"
#endif

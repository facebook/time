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

#include <chrono>
#include <iostream>
#include <vector>
#include "time/fbclock/fbclock.h"

int main() {
  std::vector<uint32_t>
      time_histogram; // create a histogramm of call time in us
  fbclock_lib fbclock = {};

  time_histogram.resize(1001, 0);

  int res = fbclock_init(&fbclock, FBCLOCK_PATH);
  if (res != 0) {
    std::cout << "Failed to init fbclock library: errno " << res << std::endl;
    exit(0);
  }
  for (int i = 0; i < 1000000; i++) {
    fbclock_truetime true_time = {};
    auto start_time = std::chrono::steady_clock::now();
    fbclock_gettime_utc(&fbclock, &true_time);
    auto end_time = std::chrono::steady_clock::now();
    auto duration = std::chrono::duration_cast<std::chrono::nanoseconds>(
        end_time - start_time);
    long hist_idx;
    if (duration.count() > 1000) {
      hist_idx = duration.count() / 1000; // convert to microseconds
      hist_idx += 20;
    } else if (duration.count() > 100) {
      hist_idx = duration.count() / 100; // convert to tens of microseconds
      hist_idx += 10;
    } else {
      hist_idx = duration.count() / 10; // convert to tens of nanoseconds
    }
    if (hist_idx >= 1000) {
      hist_idx = 1000;
    }

    time_histogram[hist_idx]++;
    if (true_time.earliest_ns + 10000 <= true_time.latest_ns) {
      std::cout << "WoU is more than 10us [" << true_time.earliest_ns << ","
                << true_time.latest_ns << "] " << std::endl;
    }
  }
  std::cout << "Histogram of query time:" << std::endl;
  for (int i = 0; i < 1001; i++) {
    if (time_histogram[i]) {
      if (i <= 10) {
        std::cout << i << "0ns: " << time_histogram[i] << std::endl;
      } else if (i < 20) {
        std::cout << i - 9 << "00ns: " << time_histogram[i] << std::endl;
      } else {
        std::cout << i - 20 << "us: " << time_histogram[i] << std::endl;
      }
    }
  }
  fbclock_destroy(&fbclock);
}

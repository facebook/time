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

#include <gtest/gtest.h>
#include <unistd.h>

#include "../fbclock.h"

namespace {

// The layout assumes a 64-byte cache line; this fails on a host whose L1 data
// cache line is another size.
TEST(FBClockDataLayoutTest, SectionIsOneCacheLine) {
  const auto line = sysconf(_SC_LEVEL1_DCACHE_LINESIZE);
  if (line <= 0) {
    GTEST_SKIP() << "this host doesn't report its cache line size";
  }
  EXPECT_EQ(sizeof(fbclock_data_section), static_cast<size_t>(line));
}

// Bit k is sections[k], line k + 1, at offset 64 * (k + 1); anything but one
// bit has no section.
TEST(FBClockDataLayoutTest, EachSourceBitIsItsSection) {
  EXPECT_EQ(fbclock_data_section_index(FBCLOCK_SOURCE_PTP), 0);
  EXPECT_EQ(fbclock_data_section_index(FBCLOCK_SOURCE_PRTC), 1);
  EXPECT_EQ(fbclock_data_section_index(FBCLOCK_SOURCE_NTP), 2);
  EXPECT_EQ(fbclock_data_section_index(1u << 31), 31);
  for (uint32_t source :
       {0u, FBCLOCK_SOURCE_PTP | FBCLOCK_SOURCE_NTP, FBCLOCK_SOURCE_ANY}) {
    EXPECT_EQ(fbclock_data_section_index(source), -1) << source;
  }
  for (uint32_t k = 0; k < 32; k++) {
    const auto offset =
        offsetof(fbclock_data, sections) + k * sizeof(fbclock_data_section);
    EXPECT_EQ(offset, 64 * (k + 1)) << k;
  }
}

} // namespace

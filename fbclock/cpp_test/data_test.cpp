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
#include <signal.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/wait.h>
#include <unistd.h>
#include <chrono>
#include <new>

#include "../fbclock.h"

namespace {

using SteadyClock = std::chrono::steady_clock;

// A PTP host's sources, and the same host once it also offers NTP.
constexpr uint32_t kPtpHost = FBCLOCK_SOURCE_PTP | FBCLOCK_SOURCE_PRTC;
constexpr uint32_t kPtpAndNtpHost = kPtpHost | FBCLOCK_SOURCE_NTP;

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

// Waits give up after this, so a broken writer fails a test instead of hanging.
SteadyClock::time_point deadline() {
  return SteadyClock::now() + std::chrono::seconds(10);
}

bool waitForFlag(
    const atomic_uint64* flag,
    uint64_t value,
    SteadyClock::time_point until) {
  while (atomic_load(flag) != value) {
    if (SteadyClock::now() >= until) {
      return false;
    }
  }
  return true;
}

bool waitForMagic(
    const fbclock_data_header* header,
    fbclock_data_header* copy,
    SteadyClock::time_point until) {
  fbclock_data_header_load(header, copy);
  while (memcmp(copy->magic, FBCLOCK_DATA_MAGIC, sizeof(copy->magic))) {
    if (SteadyClock::now() >= until) {
      return false;
    }
    fbclock_data_header_load(header, copy);
  }
  return true;
}

fbclock_data_section* sectionOf(fbclock_data* data, uint32_t source) {
  return &data->sections[fbclock_data_section_index(source)];
}

fbclock_data_clock clockFor(int64_t generation) {
  return {
      .ref_time_ns = generation * 2,
      .sysclock_time_ns = generation * 3,
      .coef_ppb = -generation * 4,
      .clock_id = CLOCK_MONOTONIC_RAW,
      .error_bound_ns = static_cast<uint32_t>(generation * 5),
      .ingress_time_ns = generation,
      .holdover_multiplier_ns = static_cast<uint32_t>(generation * 6),
  };
}

class FBClockDataTest : public testing::Test {
 protected:
  void SetUp() override {
    file_ = tmpfile();
    ASSERT_NE(file_, nullptr);
    ASSERT_EQ(ftruncate(fileno(file_), FBCLOCK_DATA_SIZE), 0);
    writer_ = static_cast<fbclock_data*>(mmap(
        nullptr,
        FBCLOCK_DATA_SIZE,
        PROT_READ | PROT_WRITE,
        MAP_SHARED,
        fileno(file_),
        0));
    ASSERT_NE(writer_, MAP_FAILED);
    reader_ = static_cast<fbclock_data*>(mmap(
        nullptr, FBCLOCK_DATA_SIZE, PROT_READ, MAP_SHARED, fileno(file_), 0));
    ASSERT_NE(reader_, MAP_FAILED);
    // Shared with a forked writer, outside the file under test.
    flags_ = static_cast<atomic_uint64*>(mmap(
        nullptr,
        kFlagsSize,
        PROT_READ | PROT_WRITE,
        MAP_SHARED | MAP_ANONYMOUS,
        -1,
        0));
    ASSERT_NE(flags_, MAP_FAILED);
    for (size_t i = 0; i < kFlags; i++) {
      new (&flags_[i]) atomic_uint64(0);
    }
  }

  void TearDown() override {
    if (flags_ != nullptr && flags_ != MAP_FAILED) {
      munmap(flags_, kFlagsSize);
    }
    if (reader_ != nullptr && reader_ != MAP_FAILED) {
      munmap(reader_, FBCLOCK_DATA_SIZE);
    }
    if (writer_ != nullptr && writer_ != MAP_FAILED) {
      munmap(writer_, FBCLOCK_DATA_SIZE);
    }
    if (file_ != nullptr) {
      fclose(file_);
    }
  }

  static constexpr size_t kFlags = 2;
  static constexpr size_t kFlagsSize = kFlags * sizeof(atomic_uint64);
  FILE* file_ = nullptr;
  fbclock_data* writer_ = nullptr;
  fbclock_data* reader_ = nullptr;
  atomic_uint64* flags_ = nullptr;
};

TEST_F(FBClockDataTest, StoreOnlyChangesItsSection) {
  auto* writer = sectionOf(writer_, FBCLOCK_SOURCE_NTP);
  auto* reader = sectionOf(reader_, FBCLOCK_SOURCE_NTP);
  // Reserved bytes are copied as given, so a field a later minor adds survives.
  auto value = clockFor(7);
  memset(value.reserved, 0xcd, sizeof(value.reserved));
  memset(writer->clock.reserved, 0xff, sizeof(writer->clock.reserved));
  fbclock_data_section_store(writer, &value);
  fbclock_data_clock copy = {};
  ASSERT_EQ(fbclock_data_section_load(reader, &copy), 0);
  EXPECT_EQ(atomic_load(&reader->seq), 2);
  EXPECT_EQ(memcmp(&copy, &value, sizeof(copy)), 0);
  const auto* bytes = reinterpret_cast<const uint8_t*>(reader_);
  const size_t ntpBegin = reinterpret_cast<const uint8_t*>(reader) - bytes;
  const size_t ntpEnd = ntpBegin + sizeof(*reader);
  for (size_t i = 0; i < FBCLOCK_DATA_SIZE; i++) {
    if (i < ntpBegin || i >= ntpEnd) {
      EXPECT_EQ(bytes[i], 0) << "byte " << i;
    }
  }
  fbclock_data_section_store(writer, &value);
  ASSERT_EQ(fbclock_data_section_load(reader, &copy), 0);
  EXPECT_EQ(atomic_load(&reader->seq), 4);
}

TEST_F(FBClockDataTest, UninitializedAndBusySectionsAreRefused) {
  auto* writer = sectionOf(writer_, FBCLOCK_SOURCE_PTP);
  auto* reader = sectionOf(reader_, FBCLOCK_SOURCE_PTP);
  writer->clock = clockFor(1);
  // A failed load leaves the caller's record as it was.
  fbclock_data_clock copy;
  memset(&copy, 0xab, sizeof(copy));
  const auto untouched = copy;
  EXPECT_EQ(fbclock_data_section_load(reader, &copy), FBCLOCK_E_NO_DATA);
  EXPECT_EQ(memcmp(&copy, &untouched, sizeof(copy)), 0);
  atomic_store(&writer->seq, 3);
  EXPECT_EQ(fbclock_data_section_load(reader, &copy), FBCLOCK_E_CRC_MISMATCH);
  EXPECT_EQ(memcmp(&copy, &untouched, sizeof(copy)), 0);
}

TEST_F(FBClockDataTest, SequenceWrapNeverPublishesZero) {
  auto* writer = sectionOf(writer_, FBCLOCK_SOURCE_PTP);
  auto* reader = sectionOf(reader_, FBCLOCK_SOURCE_PTP);
  atomic_store(&writer->seq, UINT64_MAX - 1);
  auto value = clockFor(1);
  fbclock_data_section_store(writer, &value);
  fbclock_data_clock copy = {};
  ASSERT_EQ(fbclock_data_section_load(reader, &copy), 0);
  EXPECT_EQ(atomic_load(&reader->seq), 2);
  EXPECT_EQ(memcmp(&copy, &value, sizeof(copy)), 0);
}

TEST_F(FBClockDataTest, HeaderReplacesOldVersionAndReservedBytes) {
  memset(&writer_->header, 0xff, sizeof(writer_->header));
  fbclock_data_header_store(&writer_->header, kPtpHost);
  fbclock_data_header expected = {
      .magic = {'F', 'B', 'C', 'L', 'O', 'C', 'K', '\0'},
      .major = FBCLOCK_DATA_MAJOR,
      .minor = FBCLOCK_DATA_MINOR,
      .sources = kPtpHost,
  };
  fbclock_data_header copy;
  memset(&copy, 0xab, sizeof(copy));
  fbclock_data_header_load(&reader_->header, &copy);
  EXPECT_EQ(memcmp(&copy, &expected, sizeof(expected)), 0);
  EXPECT_EQ(atomic_load(&sectionOf(reader_, FBCLOCK_SOURCE_PTP)->seq), 0);
}

TEST_F(FBClockDataTest, ConcurrentWriterNeverProducesATornSnapshot) {
  auto* writer = sectionOf(writer_, FBCLOCK_SOURCE_PTP);
  auto* reader = sectionOf(reader_, FBCLOCK_SOURCE_PTP);
  auto value = clockFor(1);
  fbclock_data_section_store(writer, &value);
  auto* done = &flags_[0];
  auto* seen = &flags_[1];
  pid_t child = fork();
  if (child == -1) {
    ADD_FAILURE() << "fork failed";
    return;
  }
  if (child == 0) {
    // Long enough that a missing retry lets a torn copy through on every run.
    for (int64_t i = 2; i <= 2000000 || atomic_load(seen) == 0; i++) {
      auto next = clockFor(i);
      fbclock_data_section_store(writer, &next);
    }
    atomic_store(done, 1);
    _exit(0);
  }
  int successful = 0;
  int concurrent = 0;
  bool consistent = true;
  const auto until = deadline();
  while ((atomic_load(done) == 0 || successful < 1000) &&
         SteadyClock::now() < until) {
    fbclock_data_clock copy = {};
    int result = fbclock_data_section_load(reader, &copy);
    if (result != 0) {
      consistent &= result == FBCLOCK_E_CRC_MISMATCH;
      continue;
    }
    auto expected = clockFor(copy.ingress_time_ns);
    consistent &= memcmp(&copy, &expected, sizeof(copy)) == 0;
    successful++;
    if (copy.ingress_time_ns > 1 && atomic_load(done) == 0) {
      concurrent++;
      atomic_store(seen, 1);
    }
  }
  const bool finished = atomic_load(done) != 0 && successful >= 1000;
  if (!finished) {
    kill(child, SIGKILL);
  }
  int status = 0;
  ASSERT_EQ(waitpid(child, &status, 0), child);
  EXPECT_TRUE(finished);
  EXPECT_EQ(status, 0);
  EXPECT_TRUE(consistent);
  EXPECT_GT(concurrent, 0);
}

TEST_F(FBClockDataTest, MagicPublishesTheInitializedHeader) {
  auto* phase = &flags_[0];
  constexpr uint64_t iterations = 1000;
  pid_t child = fork();
  if (child == -1) {
    ADD_FAILURE() << "fork failed";
    return;
  }
  if (child == 0) {
    for (uint64_t i = 0; i < iterations; i++) {
      memset(writer_->header.magic, 0, sizeof(writer_->header.magic));
      writer_->header.major = 0xff;
      writer_->header.minor = 0xff;
      memset(
          writer_->header.reserved0, 0xff, sizeof(writer_->header.reserved0));
      writer_->header.sources = UINT32_MAX;
      memset(writer_->header.reserved, 0xff, sizeof(writer_->header.reserved));
      atomic_store(phase, 3 * i + 1);
      while (atomic_load(phase) != 3 * i + 2) {
      }
      fbclock_data_header_store(&writer_->header, kPtpHost);
      while (atomic_load(phase) != 3 * i + 3) {
      }
    }
    _exit(0);
  }
  bool initialized = true;
  bool published = true;
  const auto until = deadline();
  for (uint64_t i = 0; i < iterations; i++) {
    if (!waitForFlag(phase, 3 * i + 1, until)) {
      published = false;
      break;
    }
    atomic_store(phase, 3 * i + 2);
    fbclock_data_header copy;
    if (!waitForMagic(&reader_->header, &copy, until)) {
      published = false;
      break;
    }
    initialized &= copy.major == FBCLOCK_DATA_MAJOR;
    initialized &= copy.minor == FBCLOCK_DATA_MINOR;
    initialized &= copy.sources == kPtpHost;
    for (auto byte : copy.reserved0) {
      initialized &= byte == 0;
    }
    for (auto byte : copy.reserved) {
      initialized &= byte == 0;
    }
    atomic_store(phase, 3 * i + 3);
  }
  if (!published) {
    kill(child, SIGKILL);
  }
  int status = 0;
  ASSERT_EQ(waitpid(child, &status, 0), child);
  EXPECT_TRUE(published);
  EXPECT_EQ(status, 0);
  EXPECT_TRUE(initialized);
}

// A restart rewrites a valid header (new minor, NTP toggled); readers opening
// meanwhile see the magic, the major, and whole minor and sources values.
TEST_F(FBClockDataTest, RewritingAValidHeaderStaysValid) {
  fbclock_data_header_store(&writer_->header, kPtpHost);
  auto* stop = &flags_[0];
  auto* stores = &flags_[1];
  pid_t child = fork();
  if (child == -1) {
    ADD_FAILURE() << "fork failed";
    return;
  }
  if (child == 0) {
    for (uint64_t i = 1; atomic_load(stop) == 0; i++) {
      writer_->header.minor = FBCLOCK_DATA_MINOR + 1;
      fbclock_data_header_store(
          &writer_->header, i % 2 ? kPtpAndNtpHost : kPtpHost);
      atomic_store(stores, i);
    }
    _exit(0);
  }
  bool valid = true;
  const auto until = deadline();
  while (atomic_load(stores) < 100000 && SteadyClock::now() < until) {
    fbclock_data_header copy;
    fbclock_data_header_load(&reader_->header, &copy);
    valid &= memcmp(copy.magic, FBCLOCK_DATA_MAGIC, 8) == 0;
    valid &= copy.major == FBCLOCK_DATA_MAJOR;
    valid &= copy.minor == FBCLOCK_DATA_MINOR ||
        copy.minor == FBCLOCK_DATA_MINOR + 1;
    valid &= copy.sources == kPtpHost || copy.sources == kPtpAndNtpHost;
    for (auto byte : copy.reserved) {
      valid &= byte == 0;
    }
  }
  const bool finished = atomic_load(stores) >= 100000;
  atomic_store(stop, 1);
  int status = 0;
  ASSERT_EQ(waitpid(child, &status, 0), child);
  EXPECT_TRUE(finished);
  EXPECT_EQ(status, 0);
  EXPECT_TRUE(valid);
}

} // namespace

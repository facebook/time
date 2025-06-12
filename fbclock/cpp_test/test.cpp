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
#include <stdio.h>
#include <sys/mman.h>
#include <cmath>
#include <future>
#include <thread>

#include "../fbclock.h"

TEST(fbclockTest, test_write_read) {
  int err;
  char* test_shm = std::tmpnam(nullptr);

  // open file, write data into it
  FILE* f = fopen(test_shm, "wb+");
  int sfd_rw = fileno(f);
  ASSERT_NE(sfd_rw, -1);

  err = ftruncate(sfd_rw, FBCLOCK_SHMDATA_SIZE);
  ASSERT_EQ(err, 0);

  fbclock_clockdata data = {
      .ingress_time_ns = 1, .error_bound_ns = 2, .holdover_multiplier_ns = 3};
  err = fbclock_clockdata_store_data(sfd_rw, &data);
  ASSERT_EQ(err, 0);

  fclose(f);

  // read data from the file
  f = fopen(test_shm, "r");
  int sfd_ro = fileno(f);
  ASSERT_NE(sfd_ro, -1);

  fbclock_shmdata* shmp = (fbclock_shmdata*)mmap(
      nullptr, FBCLOCK_SHMDATA_SIZE, PROT_READ, MAP_SHARED, sfd_ro, 0);
  ASSERT_NE(shmp, MAP_FAILED);

  fbclock_clockdata read_data;

  err = fbclock_clockdata_load_data(shmp, &read_data);
  ASSERT_EQ(err, 0);

  munmap(shmp, FBCLOCK_SHMDATA_SIZE);
  fclose(f);

  EXPECT_EQ(data.ingress_time_ns, read_data.ingress_time_ns);
  EXPECT_EQ(data.error_bound_ns, read_data.error_bound_ns);
  EXPECT_EQ(data.holdover_multiplier_ns, read_data.holdover_multiplier_ns);

  remove(test_shm);
}

int writer_thread(int sfd_rw, int tries) {
  int err;
  fbclock_clockdata data = {
      .ingress_time_ns = 1, .error_bound_ns = 2, .holdover_multiplier_ns = 3};
  for (int i = 0; i < tries; i++) {
    err = fbclock_clockdata_store_data(sfd_rw, &data);
    if (err != 0) {
      return err;
    }
    data.ingress_time_ns = data.ingress_time_ns + 1;
    if (data.ingress_time_ns > 10000) {
      data.ingress_time_ns = 1;
    }
    data.error_bound_ns = data.ingress_time_ns * 2;
    data.holdover_multiplier_ns = data.ingress_time_ns * 3;
  }
  return 0;
}

int reader_thread(fbclock_shmdata* shmp, int tries) {
  int err;
  fbclock_clockdata data;
  for (int i = 0; i < tries; i++) {
    err = fbclock_clockdata_load_data(shmp, &data);
    if (err != 0) {
      return err;
    }
    if (data.ingress_time_ns * 2 != data.error_bound_ns) {
      printf("ingress_time_ns: %lu\n", data.ingress_time_ns);
      printf("error_bound_ns: %d\n", data.error_bound_ns);
      printf("holdover_multiplier_ns: %d\n", data.holdover_multiplier_ns);
      return -1;
    }
    if (data.ingress_time_ns * 3 != data.holdover_multiplier_ns) {
      printf("ingress_time_ns: %lu\n", data.ingress_time_ns);
      printf("error_bound_ns: %d\n", data.error_bound_ns);
      printf("holdover_multiplier_ns: %d\n", data.holdover_multiplier_ns);
      return -1;
    }
  }
  return 0;
}

TEST(fbclockTest, test_concurrent) {
  int err;
  char* test_shm = std::tmpnam(nullptr);

  // open file, write data into it
  FILE* f_rw = fopen(test_shm, "wb+");
  int sfd_rw = fileno(f_rw);
  ASSERT_NE(sfd_rw, -1);

  err = ftruncate(sfd_rw, FBCLOCK_SHMDATA_SIZE);
  ASSERT_EQ(err, 0);

  // read data from the file
  FILE* f_ro = fopen(test_shm, "r");
  int sfd_ro = fileno(f_ro);
  ASSERT_NE(sfd_ro, -1);

  fbclock_shmdata* shmp = (fbclock_shmdata*)mmap(
      nullptr, FBCLOCK_SHMDATA_SIZE, PROT_READ, MAP_SHARED, sfd_ro, 0);
  ASSERT_NE(shmp, MAP_FAILED);

  int tries = 10000;

  // spawn two functions asynchronously, make sure there is no inconsistent data
  auto future_writer =
      std::async(std::launch::async, writer_thread, sfd_rw, tries);
  auto future_reader =
      std::async(std::launch::async, reader_thread, shmp, tries);
  err = future_writer.get();
  ASSERT_EQ(err, 0);
  err = future_reader.get();
  ASSERT_EQ(err, 0);
  munmap(shmp, FBCLOCK_SHMDATA_SIZE);
  remove(test_shm);
}

int writer_thread_v2(int sfd_rw, int tries) {
  int err;
  fbclock_clockdata_v2 data = {
      .ingress_time_ns = 1,
      .error_bound_ns = 2,
      .holdover_multiplier_ns = 3,
      .clockId = CLOCK_MONOTONIC_RAW,
      .phc_time_ns = 1748164346441310791,
      .sysclock_time_ns = 1748164309441310791,
  };
  for (int i = 0; i < tries; i++) {
    err = fbclock_clockdata_store_data_v2(sfd_rw, &data);
    if (err != 0) {
      return err;
    }
    data.ingress_time_ns = data.ingress_time_ns + 1000;
    if (data.ingress_time_ns > 10000) {
      data.ingress_time_ns = 1;
    }
    data.error_bound_ns = data.ingress_time_ns * 2;
    data.holdover_multiplier_ns = data.ingress_time_ns * 3;
    data.phc_time_ns += 10000;
    data.sysclock_time_ns += 10000;
    usleep(10000); // sleep 10ms - this will be the normal case
  }
  return 0;
}

int reader_thread_v2(fbclock_shmdata_v2* shmp, int tries) {
  int err;
  fbclock_clockdata_v2 data;
  for (int i = 0; i < tries; i++) {
    err = fbclock_clockdata_load_data_v2(shmp, &data);
    if (err != 0) {
      printf("load v2 data failed: %d\n", err);
      return err;
    }
    if (data.ingress_time_ns * 2 != data.error_bound_ns) {
      printf("ingress_time_ns: %lu\n", data.ingress_time_ns);
      printf("error_bound_ns: %d\n", data.error_bound_ns);
      printf("holdover_multiplier_ns: %d\n", data.holdover_multiplier_ns);
      return -1;
    }
    if (data.ingress_time_ns * 3 != data.holdover_multiplier_ns) {
      printf("ingress_time_ns: %lu\n", data.ingress_time_ns);
      printf("error_bound_ns: %d\n", data.error_bound_ns);
      printf("holdover_multiplier_ns: %d\n", data.holdover_multiplier_ns);
      return -2;
    }
    if ((data.phc_time_ns - data.sysclock_time_ns) != 37000000000) {
      printf("phc_time_ns: %lu\n", data.phc_time_ns);
      printf("sysclock_time_ns: %lu\n", data.sysclock_time_ns);
      return -3;
    }
  }
  return 0;
}

TEST(fbclockTest, test_concurrent_v2) {
  int err;
  char* test_shm = std::tmpnam(nullptr);

  // open file, write data into it
  FILE* f_rw = fopen(test_shm, "wb+");
  int sfd_rw = fileno(f_rw);
  ASSERT_NE(sfd_rw, -1);

  err = ftruncate(sfd_rw, FBCLOCK_SHMDATA_V2_SIZE);
  ASSERT_EQ(err, 0);

  // read data from the file
  FILE* f_ro = fopen(test_shm, "r");
  int sfd_ro = fileno(f_ro);
  ASSERT_NE(sfd_ro, -1);

  fbclock_shmdata_v2* shmp = (fbclock_shmdata_v2*)mmap(
      nullptr, FBCLOCK_SHMDATA_V2_SIZE, PROT_READ, MAP_SHARED, sfd_ro, 0);
  ASSERT_NE(shmp, MAP_FAILED);

  int tries = 1000;

  // spawn two functions asynchronously, make sure there is no inconsistent data
  auto future_writer =
      std::async(std::launch::async, writer_thread_v2, sfd_rw, tries);
  auto future_reader =
      std::async(std::launch::async, reader_thread_v2, shmp, tries * 10);
  err = future_writer.get();
  ASSERT_EQ(err, 0);
  err = future_reader.get();
  ASSERT_EQ(err, 0);
  munmap(shmp, FBCLOCK_SHMDATA_V2_SIZE);
  remove(test_shm);
}

TEST(fbclockTest, test_window_of_uncertainty) {
  int64_t seconds = 0; // how long ago was the last SYNC
  double error_bound_ns = 172.0;
  double holdover_multiplier_ns = 50.5;

  double wou = fbclock_window_of_uncertainty(
      seconds, error_bound_ns, holdover_multiplier_ns);
  EXPECT_DOUBLE_EQ(wou, 172.0);

  seconds = 10;
  wou = fbclock_window_of_uncertainty(
      seconds, error_bound_ns, holdover_multiplier_ns);

  EXPECT_DOUBLE_EQ(wou, 677.0);
}

TEST(fbclockTest, test_fbclock_calculate_time) {
  int err;
  fbclock_truetime truetime;
  fbclock_clockdata state = {
      .ingress_time_ns = 1647269091803102957,
  };
  double error_bound = 172.0;
  double h_value = 50.5;
  // phc time is before ingress time, error
  int64_t phctime_ns = 1647269082943150996;

  err = fbclock_calculate_time(
      error_bound, h_value, &state, phctime_ns, &truetime, FBCLOCK_TAI);
  ASSERT_EQ(err, FBCLOCK_E_PHC_IN_THE_PAST);

  // phc time is after ingress time, all good
  state = {.ingress_time_ns = 1647269082943150996};
  phctime_ns = 1647269091803102957;
  err = fbclock_calculate_time(
      error_bound, h_value, &state, phctime_ns, &truetime, FBCLOCK_TAI);
  ASSERT_EQ(err, 0);

  EXPECT_EQ(truetime.earliest_ns, 1647269091803102338);
  EXPECT_EQ(truetime.latest_ns, 1647269091803103576);

  // WOU is very big
  error_bound = 1000.0;
  phctime_ns += 6 * 3600 * 1000000000.0; // + 6 hours
  err = fbclock_calculate_time(
      error_bound, h_value, &state, phctime_ns, &truetime, FBCLOCK_TAI);
  ASSERT_EQ(err, 0);
  EXPECT_EQ(truetime.earliest_ns, 1647290691802010729);
  EXPECT_EQ(truetime.latest_ns, 1647290691804195223);
}

TEST(fbclockTest, test_fbclock_calculate_time_v2) {
  int err;
  fbclock_truetime truetime;
  fbclock_clockdata_v2 state = {
      .ingress_time_ns = 1647269091803102957,
      .clockId = CLOCK_MONOTONIC_RAW,
      // phc time is before ingress time, error
      .phc_time_ns = 1647269082943150996,
  };
  double error_bound = 172.0;
  double h_value = 50.5;

  struct timespec tp = {};
  clock_gettime(CLOCK_MONOTONIC_RAW, &tp);

  int64_t sysclock_time_ns = tp.tv_sec * 1000000000 + tp.tv_nsec;
  state.sysclock_time_ns = sysclock_time_ns;

  err = fbclock_calculate_time_v2(
      error_bound,
      h_value,
      &state,
      sysclock_time_ns + 1000, // + 1us
      &truetime,
      FBCLOCK_TAI);
  ASSERT_EQ(err, FBCLOCK_E_PHC_IN_THE_PAST);

  // phc time is after ingress time, all good
  state = {
      .ingress_time_ns = 1647269082943150996,
      .clockId = CLOCK_MONOTONIC_RAW,
      .phc_time_ns = 1647269091803102957,
      .sysclock_time_ns = sysclock_time_ns,
      .coef_ppb = 12,
  };
  err = fbclock_calculate_time_v2(
      error_bound,
      h_value,
      &state,
      sysclock_time_ns + 1000,
      &truetime,
      FBCLOCK_TAI);
  ASSERT_EQ(err, 0);

  EXPECT_EQ(truetime.earliest_ns, 1647269091803103338);
  EXPECT_EQ(truetime.latest_ns, 1647269091803104576);

  // WOU is very big
  error_bound = 1000.0;
  sysclock_time_ns += 6 * 3600 * 1000000000ULL; // + 6 hours
  err = fbclock_calculate_time_v2(
      error_bound, h_value, &state, sysclock_time_ns, &truetime, FBCLOCK_TAI);
  ASSERT_EQ(err, 0);
  EXPECT_EQ(truetime.earliest_ns, 1647290691803360710);
  EXPECT_EQ(truetime.latest_ns, 1647290691803363604);
}

TEST(fbclockTest, test_fbclock_apply_smear_after_2017_leap_second) {
  uint64_t offset_pre_ns = 36e9;
  uint64_t offset_post_ns = 37e9;
  uint64_t smear_start_ns = 1483228836e9; // Sun, 01 Jan 2017 00:00:36 TAI
  uint64_t smear_end_ns = 1483293836e9; // Sun, 01 Jan 2017 18:03:56 TAI
  int multiplier = 1;

  uint64_t time =
      1714142307961569530; // Friday, 26 April 2024 14:38:27.961:569:530 TAI
  time = fbclock_apply_smear(
      time,
      offset_pre_ns,
      offset_post_ns,
      smear_start_ns,
      smear_end_ns,
      multiplier);
  // Expect UTC time to be 37 seconds behind TAI
  EXPECT_EQ(
      time,
      1714142270961569530); // Friday, 26 April 2024 14:37:50.961:569:530 UTC

  time = 1714142307961570584; // Friday, 26 April 2024 14:38:27.961:570:584 TAI
  time = fbclock_apply_smear(
      time,
      offset_pre_ns,
      offset_post_ns,
      smear_start_ns,
      smear_end_ns,
      multiplier);
  // Expect UTC time to be 37 seconds behind TAI
  EXPECT_EQ(
      time,
      1714142270961570584); // Friday, 26 April 2024 14:37:50.961:570:584 UTC
}

TEST(fbclockTest, test_fbclock_apply_smear_before_2017_leap_second) {
  uint64_t offset_pre_ns = 36e9;
  uint64_t offset_post_ns = 37e9;
  uint64_t smear_start_ns = 1483228836e9; // Sun, 01 Jan 2017 00:00:36 TAI
  uint64_t smear_end_ns = 1483293836e9; // Sun, 01 Jan 2017 18:03:56 TAI
  int multiplier = 1;

  uint64_t time =
      1443142307961555444; // Friday, 25 Sep 2015 00:51:47.961:555:444 TAI
  time = fbclock_apply_smear(
      time,
      offset_pre_ns,
      offset_post_ns,
      smear_start_ns,
      smear_end_ns,
      multiplier);
  // Expect UTC time to be 36 seconds behind TAI
  EXPECT_EQ(
      time,
      1443142271961555444); // Friday, 25 Sep 2015 00:51:11.961:555:444 UTC

  time = 1443142308666555444; // Friday, 25 Sep 2015 00:51:48.666:555:444 TAI
  time = fbclock_apply_smear(
      time,
      offset_pre_ns,
      offset_post_ns,
      smear_start_ns,
      smear_end_ns,
      multiplier);
  // Expect UTC time to be 36 seconds behind TAI
  EXPECT_EQ(
      time,
      1443142272666555444); // Friday, 25 Sep 2015 00:51:12.666:555:444 UTC
}

TEST(fbclockTest, test_fbclock_apply_smear_during_2017_leap_second_params) {
  uint64_t offset_pre_ns = 36e9;
  uint64_t offset_post_ns = 37e9;
  uint64_t smear_start_ns = 1483228836e9; // Sun, 01 Jan 2017 00:00:36 TAI
  uint64_t smear_end_ns = 1483293836e9; // Sun, 01 Jan 2017 18:03:56 TAI
  int multiplier = 1;

  uint64_t input_times[] = {
      1483228835000000000, // Sun, 01 Jan 2017 00:00:35:000:000:000 TAI
      1483228836000000000, // Sun, 01 Jan 2017 00:00:36:000:000:000 TAI (start)
      1483228836000065000, // Sun, 01 Jan 2017 00:00:36:000:065:000 TAI
      1483228836000130000, // Sun, 01 Jan 2017 00:00:36:000:130:000 TAI
      1483228837000000000, // Sun, 01 Jan 2017 00:00:37:000:000:000 TAI
      1483261335000000000, // Sun, 01 Jan 2017 09:02:15:000:000:000 TAI
      1483261336000000000, // Sun, 01 Jan 2017 09:02:16:000:000:000 TAI
                           // (midpoint)
      1483261337000000000, // Sun, 01 Jan 2017 09:02:17:000:000:000 TAI
      1483261345000000000, // Sun, 01 Jan 2017 09:02:25:000:000:000 TAI
      1483261346000000000, // Sun, 01 Jan 2017 09:02:26:000:000:000 TAI
      1483261347000000000, // Sun, 01 Jan 2017 09:02:27:000:000:000 TAI
      1483293836000000000, // Sun, 01 Jan 2017 18:03:56:000:000:000 TAI (end)
      1483293837000000000, // Sun, 01 Jan 2017 18:03:57:000:000:000 TAI
  };

  uint64_t output_times[] = {
      1483228799000000000, // Sat, 31 Dec 2016 23:59:59:000:000:000 UTC
      1483228800000000000, // Sun, 01 Jan 2017 00:00:00:000:000:000 UTC (start)
      1483228800000064999, // Sun, 01 Jan 2017 00:00:00:000:064:999 UTC
      1483228800000129998, // Sun, 01 Jan 2017 00:00:00:000:129:998 UTC
      1483228800999984616, // Sun, 01 Jan 2017 00:00:00:999:984:616 UTC
      1483261298500015385, // Sun, 01 Jan 2017 09:01:38:500:015:385 UTC
      1483261299500000000, // Sun, 01 Jan 2017 09:01:39:500:000:000 UTC
                           // (midpoint)
      1483261300499984616, // Sun, 01 Jan 2017 09:01:40:499:984:616 UTC
      1483261308499861539, // Sun, 01 Jan 2017 09:01:49:499:861:539 UTC
      1483261309499846154, // Sun, 01 Jan 2017 09:01:49:499:846:154 UTC
      1483261310499830770, // Sun, 01 Jan 2017 09:01:50:499:830:770 UTC
      1483293799000000000, // Sun, 01 Jan 2017 18:03:19:000:000:000 UTC (end)
      1483293800000000000, // Sun, 01 Jan 2017 18:03:20:000:000:000 UTC
  };

  for (int i = 0; i < 11; ++i) {
    EXPECT_EQ(
        fbclock_apply_smear(
            input_times[i],
            offset_pre_ns,
            offset_post_ns,
            smear_start_ns,
            smear_end_ns,
            multiplier),
        output_times[i]);
  }
}

TEST(fbclockTest, test_fbclock_apply_smear_during_future_leap_second_negative) {
  uint64_t offset_pre_ns = 37e9;
  uint64_t offset_post_ns = 36e9;
  uint64_t smear_start_ns = 1893456037e9; // Sun, 01 Jan 2030 00:00:37 TAI
  uint64_t smear_end_ns = 1893521037e9; // Sun, 01 Jan 2017 18:03:57 TAI
  int multiplier = -1;

  uint64_t input_times[] = {
      1893456037000000000, // Wed, 01 Jan 2030 00:00:37:000:000:000 TAI (start)
      1893488537000000000, // Wed, 01 Jan 2030 09:02:17:000:000:000 TAI
                           // (midpoint)
      1893521037000000000, // Wed, 01 Jan 2030 18:03:57:000:000:000 TAI (end)

  };

  uint64_t output_times[] = {
      1893456000000000000, // Wed, 01 Jan 2030 00:00:00:000:000:000 UTC (start)
      1893488500500000000, // Wed, 01 Jan 2030 09:01:40.500:000:000 UTC
                           // (midpoint)
      1893521001000000000, // Wed, 01 Jan 2030 18:03:21:000:000:000 UTC (end)
  };

  for (int i = 0; i < 3; ++i) {
    EXPECT_EQ(
        fbclock_apply_smear(
            input_times[i],
            offset_pre_ns,
            offset_post_ns,
            smear_start_ns,
            smear_end_ns,
            multiplier),
        output_times[i]);
  }
}

int main(int argc, char** argv) {
  ::testing::InitGoogleTest(&argc, argv);
  return RUN_ALL_TESTS();
}

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

#include <future>
#include <gtest/gtest.h>
#include <stdio.h>
#include <sys/mman.h>
#include <thread>

#include "../fbclock.h"

TEST(fbclock_test, test_write_read) {
  int err;
  char *test_shm = std::tmpnam(nullptr);

  // open file, write data into it
  FILE *f = fopen(test_shm, "wb+");
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

  fbclock_shmdata *shmp = (fbclock_shmdata *)mmap(
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

int reader_thread(fbclock_shmdata *shmp, int tries) {
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

TEST(fbclock_test, test_concurrent) {
  int err;
  char *test_shm = std::tmpnam(nullptr);

  // open file, write data into it
  FILE *f_rw = fopen(test_shm, "wb+");
  int sfd_rw = fileno(f_rw);
  ASSERT_NE(sfd_rw, -1);

  err = ftruncate(sfd_rw, FBCLOCK_SHMDATA_SIZE);
  ASSERT_EQ(err, 0);

  // read data from the file
  FILE *f_ro = fopen(test_shm, "r");
  int sfd_ro = fileno(f_ro);
  ASSERT_NE(sfd_ro, -1);

  fbclock_shmdata *shmp = (fbclock_shmdata *)mmap(
      nullptr, FBCLOCK_SHMDATA_SIZE, PROT_READ, MAP_SHARED, sfd_ro, 0);
  ASSERT_NE(shmp, MAP_FAILED);

  int tries = 10000;

  // spawn two functions asynchronously, make sure there is no incosistent data
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

TEST(fbclock_test, test_window_of_uncertainty) {
  int64_t seconds = 0; // how long ago was the last SYNC
  double error_bound_ns = 172.0;
  double holdover_multiplier_ns = 50.5;

  double wou = fbclock_window_of_uncertainty(seconds, error_bound_ns,
                                             holdover_multiplier_ns);
  EXPECT_DOUBLE_EQ(wou, 172.0);

  seconds = 10;
  wou = fbclock_window_of_uncertainty(seconds, error_bound_ns,
                                      holdover_multiplier_ns);

  EXPECT_DOUBLE_EQ(wou, 677.0);
}

TEST(fbclock_test, test_fbclock_calculate_time) {
  int err;
  fbclock_truetime truetime;
  double error_bound_ns = 172.0;
  double h_value_ns = 50.5;
  // phc time is before ingress time, error
  int64_t ingress_time_ns = 1647269091803102957;
  int64_t phctime_ns = 1647269082943150996;

  err = fbclock_calculate_time(error_bound_ns, h_value_ns, ingress_time_ns,
                               phctime_ns, &truetime);
  ASSERT_EQ(err, FBCLOCK_E_PHC_IN_THE_PAST);

  // phc time is after ingress time, all good
  ingress_time_ns = 1647269082943150996;
  phctime_ns = 1647269091803102957;
  err = fbclock_calculate_time(error_bound_ns, h_value_ns, ingress_time_ns,
                               phctime_ns, &truetime);
  ASSERT_EQ(err, 0);

  EXPECT_EQ(truetime.earliest_ns, 1647269091803102381);
  EXPECT_EQ(truetime.latest_ns, 1647269091803103533);

  // WOU is very big
  error_bound_ns = 1000.0;
  phctime_ns += 6 * 3600 * 1000000000.0; // + 6 hours
  err = fbclock_calculate_time(error_bound_ns, h_value_ns, ingress_time_ns,
                               phctime_ns, &truetime);
  ASSERT_EQ(err, 0);
  EXPECT_EQ(truetime.earliest_ns, 1647290691802010772);
  EXPECT_EQ(truetime.latest_ns, 1647290691804195180);
}

int main(int argc, char **argv) {
  ::testing::InitGoogleTest(&argc, argv);
  return RUN_ALL_TESTS();
}

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

#include "../../fbclock/fbclock.h"

#include <stdio.h> // for printf and perror
#include <stdlib.h> // for EXIT_* constants
#include <unistd.h> // for sleep, getopt

void show_error(int err_code) {
  puts(fbclock_strerror(err_code));
}

int main(int argc, char* argv[]) {
  fbclock_truetime truetime = {0};
  fbclock_lib lib;
  int err;

  int fflag = 0;
  int uflag = 0;
  int c;

  while ((c = getopt(argc, argv, "fu")) != -1)
    switch (c) {
      case 'f':
        fflag = 1;
        break;
      case 'u':
        uflag = 1;
        break;
      default:
        fprintf(
            stderr,
            "Usage: %s [-f]\n  -f will print TrueTime in a loop\n",
            argv[0]);
        exit(EXIT_FAILURE);
    }

  err = fbclock_init(&lib, FBCLOCK_PATH);
  if (err != 0) {
    show_error(err);
    exit(EXIT_FAILURE);
  }

  while (1) {
    if (!uflag) {
      err = fbclock_gettime(&lib, &truetime);
    } else {
      err = fbclock_gettime_utc(&lib, &truetime);
    }
    if (err != 0) {
      show_error(err);
      exit(EXIT_FAILURE);
    }
    printf("TrueTime:\n");
    printf("\tEarliest: %lu\n", truetime.earliest_ns);
    printf("\tLatest: %lu\n", truetime.latest_ns);
    printf("\tWOU=%lu ns\n", truetime.latest_ns - truetime.earliest_ns);
    // if not asked to loop - stop
    if (!fflag) {
      break;
    }
    sleep(1);
  }

  err = fbclock_destroy(&lib);
  if (err != 0) {
    show_error(err);
    exit(EXIT_FAILURE);
  }
}

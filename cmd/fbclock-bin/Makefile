CC=gcc
CFLAGS+=-g -lrt

arch := $(shell arch)
ifeq ($(arch),x86_64)
	CFLAGS += -msse4.2
endif

fbclock-bin:
	$(CC) $(CFLAGS) -o fbclock-bin fbclock-bin.c ../../fbclock/fbclock.c

.PHONY: clean

clean:
	rm -f ./fbclock-bin

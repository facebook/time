CC=gcc
CFLAGS=-Wall -g -lrt -msse4.2 -std=gnu11

fbclock-bin:
	$(CC) $(CFLAGS) -o fbclock-bin fbclock-bin.c ../../fbclock/fbclock.c

.PHONY: clean

clean:
	rm -f ./fbclock-bin

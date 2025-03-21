CC=gcc
CFLAGS=-Wall -g -lrt -std=gnu11

fbclock-bin:
	$(CC) $(CFLAGS) -o fbclock-bin fbclock-bin.c ../../fbclock/fbclock.c

.PHONY: clean

clean:
	rm -f ./fbclock-bin

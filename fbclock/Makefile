CC=gcc
CFLAGS=-Wall -g -lrt -msse4.2 -std=gnu11
CPP=g++
CPPFLAGS=-fpermissive -lrt -lpthread -msse4.2 -lgtest -g

.PHONY: clean test

BUILDDIR ?= .

fbclock:
	mkdir -p $(BUILDDIR)
	$(CC) $(CFLAGS) -shared -o $(BUILDDIR)/fbclock.so -fPIC fbclock.c

test:
	$(CPP) $(CPPFLAGS) -o fbclock-test test/test.cpp fbclock.c
	./fbclock-test

clean:
	rm -f ./fbclock.so ./fbclock-test ./fbclock.so

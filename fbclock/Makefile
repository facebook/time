CC ?= gcc
CFLAGS += -Wall -g -std=gnu11
CPP=g++
CPPFLAGS=-fpermissive -lrt -lpthread -msse4.2 -lgtest -g
AR ?= ar
ARFLAGS=rcs

# Security hardening so the artifacts pass annocheck/Fedora hardened-build checks
# (bind-now, cf-protection, property-note). $(CFLAGS)/$(LDFLAGS) are honored so a
# distro spec (e.g. Fedora %set_build_flags) can still extend/override them.
CFLAGS += -fstack-protector-strong
LDFLAGS += -Wl,-z,now -Wl,-z,relro
LDLIBS += -lrt

arch := $(shell arch)
ifeq ($(arch),x86_64)
	CFLAGS += -msse4.2 -fcf-protection=full
endif
ifeq ($(arch),aarch64)
	CFLAGS += -mbranch-protection=standard
endif

.PHONY: clean test

BUILDDIR ?= .

fbclock:
	mkdir -p $(BUILDDIR)
	$(CC) $(CFLAGS) $(LDFLAGS) -shared -fPIC -o $(BUILDDIR)/fbclock.so fbclock.c $(LDLIBS)

fbclock-static:
	mkdir -p $(BUILDDIR)
	$(CC) $(CFLAGS) -c -fPIC -o $(BUILDDIR)/fbclock.o fbclock.c
	$(AR) $(ARFLAGS) $(BUILDDIR)/libfbclock.a $(BUILDDIR)/fbclock.o

test:
	$(CPP) $(CPPFLAGS) -o fbclock-test test/test.cpp fbclock.c
	./fbclock-test

clean:
	rm -f ./fbclock.so ./fbclock-test ./fbclock.so

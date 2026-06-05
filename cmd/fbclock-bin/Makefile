CC ?= gcc
CFLAGS += -g

# Security hardening so the binary passes annocheck/Fedora hardened-build checks
# (bind-now, pie, cf-protection, property-note). $(CFLAGS)/$(LDFLAGS) are honored
# so a distro spec (e.g. Fedora %set_build_flags) can still extend/override them.
CFLAGS += -fPIE -fstack-protector-strong
LDFLAGS += -pie -Wl,-z,now -Wl,-z,relro
LDLIBS += -lrt

arch := $(shell arch)
ifeq ($(arch),x86_64)
	CFLAGS += -msse4.2 -fcf-protection=full
endif
ifeq ($(arch),aarch64)
	CFLAGS += -mbranch-protection=standard
endif

fbclock-bin:
	$(CC) $(CFLAGS) $(LDFLAGS) -o fbclock-bin fbclock-bin.c ../../fbclock/fbclock.c $(LDLIBS)

.PHONY: clean

clean:
	rm -f ./fbclock-bin

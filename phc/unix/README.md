A temporary shim for "golang.org/x/sys/unix" until v0.27.0 is cut.

We can't simply update go.mod with pre-release version of the package
because of the dependency management approach Fedora has for building
RPMs from Go sources.

.SHELLFLAGS = -ec
PACKAGES ?= mountinfo mount symlink
BINDIR ?= _build/bin
CROSS ?= linux/arm linux/arm64 linux/ppc64le linux/s390x \
	freebsd/amd64 openbsd/amd64 darwin/amd64 darwin/arm64 windows/amd64
SUDO ?= sudo -n

.PHONY: all
all: clean lint test test-local cross

.PHONY: clean
clean:
	$(RM) mount/go-local.*

.PHONY: test
test: RUN_VIA_SUDO = $(shell $(SUDO) true && echo -exec \"$(SUDO)\")
test: test-local
	for p in $(PACKAGES); do \
		(cd $$p && go test $(RUN_VIA_SUDO) -v .); \
	done

# test the mount module against the local mountinfo source code instead of the
# release specified in go.mod. This allows catching regressions / breaking
# changes in mountinfo.
.PHONY: test-local
test-local: RUN_VIA_SUDO = $(shell $(SUDO) true && echo -exec \"$(SUDO)\")
test-local:
	echo 'replace github.com/moby/sys/mountinfo => ../mountinfo' | cat mount/go.mod - > mount/go-local.mod
	cd mount \
	&& go mod download -modfile=go-local.mod \
	&& go test -modfile=go-local.mod $(RUN_VIA_SUDO) -v .
	$(RM) mount/go-local.*

.PHONY: lint
lint: $(BINDIR)/golangci-lint
	$(BINDIR)/golangci-lint version
	for p in $(PACKAGES); do \
		(cd $$p && go mod download \
		&& ../$(BINDIR)/golangci-lint run); \
	done

$(BINDIR)/golangci-lint: $(BINDIR)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(BINDIR) v1.31.0

$(BINDIR):
	mkdir -p $(BINDIR)

.PHONY: cross
cross:
	for osarch in $(CROSS); do \
		export GOOS=$${osarch%/*} GOARCH=$${osarch#*/}; \
		echo "# building for $$GOOS/$$GOARCH"; \
		for p in $(PACKAGES); do \
			(cd $$p	&& go build .); \
		done; \
	done

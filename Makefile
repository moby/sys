PACKAGES ?= atomicwriter capability mountinfo mount reexec sequential signal symlink user userns
BINDIR ?= _build/bin
CROSS ?= linux/arm linux/arm64 linux/ppc64le linux/s390x \
	freebsd/amd64 openbsd/amd64 darwin/amd64 darwin/arm64 windows/amd64
SUDO ?= sudo -n
test test-local: RUN_VIA_SUDO = $(shell $(SUDO) true && echo -exec \"$(SUDO)\")

.PHONY: all
all: clean lint test cross

.PHONY: clean
clean:
	$(RM) mount/go-local.*
	$(RM) */coverage.txt

.PHONY: foreach
foreach: ## Run $(CMD) for every package.
	@if test -z '$(CMD)'; then \
		echo 'Usage: make foreach CMD="commands to run for every package"'; \
		exit 1; \
	fi
	set -eu; \
	for p in $(PACKAGES); do \
		(cd $$p; $(CMD);) \
	done

.PHONY: test
test: test-local
test: CMD=go test $(RUN_VIA_SUDO) -v -coverprofile=coverage.txt -covermode=atomic .
test: foreach

# Some modules in this repo have interdependencies:
#  - mount depends on mountinfo
#  - atomicwrite depends on sequential
#
# The code below tests these modules against their local dependencies
# to catch regressions / breaking changes early.
.PHONY: test-local
test-local: MOD = -modfile=go-local.mod
test-local:
	echo 'replace github.com/moby/sys/mountinfo => ../mountinfo' | cat mount/go.mod - > mount/go-local.mod
	# Run go mod tidy to make sure mountinfo dependency versions are met.
	cd mount && go mod tidy $(MOD) && go test $(MOD) $(RUN_VIA_SUDO) -v .
	$(RM) mount/go-local.*
	echo 'replace github.com/moby/sys/sequential => ../sequential' | cat atomicwriter/go.mod - > atomicwriter/go-local.mod
	# Run go mod tidy to make sure sequential dependency versions are met.
	cd atomicwriter && go mod tidy $(MOD) && go test $(MOD) $(RUN_VIA_SUDO) -v .
	$(RM) atomicwriter/go-local.*

.PHONY: lint
lint: $(BINDIR)/golangci-lint
lint: CMD=go mod download; ../$(BINDIR)/golangci-lint run
lint: foreach
lint:
	$(BINDIR)/golangci-lint version

$(BINDIR)/golangci-lint: $(BINDIR)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(BINDIR) v2.0.2

$(BINDIR):
	mkdir -p $(BINDIR)

.PHONY: cross
cross:
	set -eu; \
	for osarch in $(CROSS); do \
		export GOOS=$${osarch%/*} GOARCH=$${osarch#*/}; \
		echo "# building for $$GOOS/$$GOARCH"; \
		for p in $(PACKAGES); do \
			(cd $$p; go build .); \
		done; \
	done

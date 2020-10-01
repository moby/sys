.SHELLFLAGS = -ec
PACKAGES ?= mountinfo mount
BINDIR ?= _build/bin
CROSS ?= linux/arm linux/arm64 linux/ppc64le linux/s390x \
	freebsd/amd64 openbsd/amd64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all
all: lint test cross

.PHONY: test
test:
	for p in $(PACKAGES); do \
		(cd $$p && go test -v .); \
	done

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

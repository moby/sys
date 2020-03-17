PACKAGES ?= mountinfo mount
BINDIR ?= _build/bin

.PHONY: all
all: lint test

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
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(BINDIR) v1.24.0

$(BINDIR):
	mkdir -p $(BINDIR)

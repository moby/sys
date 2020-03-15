PACKAGES ?= mountinfo mount

.PHONY: test
test:
	for p in $(PACKAGES); do \
		(cd $$p && go test -v .); \
	done

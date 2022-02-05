.SHELLFLAGS = -ec

.PHONY: fail
fail:
	for x in 1 2 3; do \
		(echo $$x && [ $$x -gt 1 ]); \
	done

.PHONY: test fmt vet check

# Run every test 100 times in a shuffled order. Deterministic simulation must
# give the same result on every run; shuffling catches tests that depend on
# each other. A failure prints the -shuffle seed, so the order can be replayed.
test:
	go test ./... -count=100 -shuffle=on

fmt:
	gofmt -w .

vet:
	go vet ./...

# Run before finishing any change.
check: fmt vet test

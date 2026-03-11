.PHONY: lint test build fuzz

FUZZTIME ?= 10s

lint:
	@which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run ./...

test:
	go test ./...

build:
	go build ./...

fuzz:
	go test ./runtime -run=^$$ -fuzz=FuzzRuntimeScript -fuzztime=$(FUZZTIME)
	go test ./runtime -run=^$$ -fuzz=FuzzMalformedScript -fuzztime=$(FUZZTIME)
	go test ./runtime -run=^$$ -fuzz=FuzzSessionSequence -fuzztime=$(FUZZTIME)
	go test ./runtime -run=^$$ -fuzz=FuzzFilePathCommands -fuzztime=$(FUZZTIME)
	go test ./runtime -run=^$$ -fuzz=FuzzTextSearchCommands -fuzztime=$(FUZZTIME)
	go test ./runtime -run=^$$ -fuzz=FuzzShellProcessCommands -fuzztime=$(FUZZTIME)
	go test ./runtime -run=^$$ -fuzz=FuzzDataCommands -fuzztime=$(FUZZTIME)
	go test ./runtime -run=^$$ -fuzz=FuzzGeneratedPrograms -fuzztime=$(FUZZTIME)
	go test ./runtime -run=^$$ -fuzz=FuzzAttackMutations -fuzztime=$(FUZZTIME)

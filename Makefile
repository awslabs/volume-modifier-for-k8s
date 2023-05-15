PROTO_FILE=modify.proto
PROTO_GENERATED_FILES_PATH=pkg/rpc
VERSION="v0.1.0"

.PHONY: all
all: build

.PHONY: build
build:
	go build -o bin/main -ldflags="-X 'main.version=$(VERSION)'" cmd/main.go

.PHONY: proto
proto:
	protoc --go_out=$(PROTO_GENERATED_FILES_PATH) --go_opt=paths=source_relative --go-grpc_out=$(PROTO_GENERATED_FILES_PATH) --go-grpc_opt=paths=source_relative $(PROTO_FILE)

.PHONY: test
test:
	go test ./... -race

.PHONY: clean
clean:
	rm -rf bin/


.PHONY: check
check: check-proto

.PHONY: check-proto
check-proto:
	$(eval TMPDIR := $(shell mktemp -d))
	protoc --go_out=$(TMPDIR) --go_opt=paths=source_relative --go-grpc_out=$(TMPDIR) --go-grpc_opt=paths=source_relative $(PROTO_FILE)
	diff -r $(TMPDIR) $(PROTO_GENERATED_FILES_PATH) || (printf "\nThe proto file seems to have been modified. PLease run `make proto`."; exit 1)
	rm -rf $(TMPDIR)
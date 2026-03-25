ifneq (,$(filter new-version,$(MAKECMDGOALS)))
  NEW_VERSION := $(word 2,$(MAKECMDGOALS))
  $(eval $(NEW_VERSION):;@:)
endif

PROTO_FILE=modify.proto
PROTO_GENERATED_FILES_PATH=pkg/rpc
# Extract from BUILD_PLATFORMS/REV to mimic csi-release-tools behavior
OS ?= $(word 1,$(BUILD_PLATFORMS))
ARCH ?= $(word 2,$(BUILD_PLATFORMS))
REV ?= "v0.9.4"
LDFLAGS="-X 'main.version=$(REV)'"
.PHONY: all
all: bin/volume-modifier-for-k8s

bin:
	@mkdir -p $@

.PHONY: proto
proto:
	protoc --go_out=$(PROTO_GENERATED_FILES_PATH) --go_opt=paths=source_relative --go-grpc_out=$(PROTO_GENERATED_FILES_PATH) --go-grpc_opt=paths=source_relative $(PROTO_FILE)

.PHONY: test
test:
	go test ./... -race

.PHONY: test/coverage
test/coverage:
	go test -coverprofile=cover.out ./cmd/... ./pkg/controller/... ./pkg/modifier/... ./pkg/util/...
	grep -vE "mock_" cover.out > filtered_cover.out
	go tool cover -func=filtered_cover.out
	go tool cover -html=filtered_cover.out -o coverage.html
	rm cover.out filtered_cover.out

.PHONY: clean
clean:
	rm -rf bin/


.PHONY: check
check: check-proto

.PHONY: linux/$(ARCH) bin/volume-modifier-for-k8s
linux/$(ARCH): bin/volume-modifier-for-k8s
bin/volume-modifier-for-k8s: | bin
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build -mod=mod -ldflags ${LDFLAGS} -o bin/volume-modifier-for-k8s ./cmd

.PHONY: new-version
new-version:
	@[ "$(NEW_VERSION)" ] || (echo "Usage: make new-version <version>" && exit 1)
	sed -i 's/^REV ?= .*/REV ?= "$(NEW_VERSION)"/' Makefile
	go get -u ./...
	go mod tidy

.PHONY: check-proto
check-proto:
	$(eval TMPDIR := $(shell mktemp -d))
	protoc --go_out=$(TMPDIR) --go_opt=paths=source_relative --go-grpc_out=$(TMPDIR) --go-grpc_opt=paths=source_relative $(PROTO_FILE)
	diff -r $(TMPDIR) $(PROTO_GENERATED_FILES_PATH) || (printf "\nThe proto file seems to have been modified. PLease run `make proto`."; exit 1)
	rm -rf $(TMPDIR)

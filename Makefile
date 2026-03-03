PROTO_FILE=modify.proto
PROTO_GENERATED_FILES_PATH=pkg/rpc
# Extract from BUILD_PLATFORMS/REV to mimic csi-release-tools behavior
OS ?= $(word 1,$(BUILD_PLATFORMS))
ARCH ?= $(word 2,$(BUILD_PLATFORMS))
REV ?= "v0.9.3"
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

.PHONY: clean
clean:
	rm -rf bin/


.PHONY: check
check: check-proto

.PHONY: linux/$(ARCH) bin/volume-modifier-for-k8s
linux/$(ARCH): bin/volume-modifier-for-k8s
bin/volume-modifier-for-k8s: | bin
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build -mod=mod -ldflags ${LDFLAGS} -o bin/volume-modifier-for-k8s ./cmd

.PHONY: check-proto
check-proto:
	$(eval TMPDIR := $(shell mktemp -d))
	protoc --go_out=$(TMPDIR) --go_opt=paths=source_relative --go-grpc_out=$(TMPDIR) --go-grpc_opt=paths=source_relative $(PROTO_FILE)
	diff -r $(TMPDIR) $(PROTO_GENERATED_FILES_PATH) || (printf "\nThe proto file seems to have been modified. PLease run `make proto`."; exit 1)
	rm -rf $(TMPDIR)

ARG BASE_IMAGE

FROM --platform=$BUILDPLATFORM golang:1.20 AS builder
WORKDIR /go/src/github.com/awslabs/volume-modifier-for-k8s
COPY go.* .
ARG GOPROXY=direct
RUN go mod download
COPY . .
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
RUN mkdir bin && OS=$TARGETOS ARCH=$TARGETARCH make $TARGETOS/$TARGETARCH

FROM $BASE_IMAGE
COPY --from=builder /go/src/github.com/awslabs/volume-modifier-for-k8s/bin/volume-modifier-for-k8s /bin/volume-modifier-for-k8s
ENTRYPOINT ["/bin/volume-modifier-for-k8s"]

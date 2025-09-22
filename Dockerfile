FROM --platform=$BUILDPLATFORM golang:1.25 AS builder
WORKDIR /go/src/github.com/awslabs/volume-modifier-for-k8s
COPY go.* .
ARG GOPROXY=direct
RUN go mod download
COPY . .
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
RUN OS=$TARGETOS ARCH=$TARGETARCH make $TARGETOS/$TARGETARCH

FROM public.ecr.aws/eks-distro-build-tooling/eks-distro-minimal-base:latest-al23 AS linux-amazon
COPY --from=builder /go/src/github.com/awslabs/volume-modifier-for-k8s/bin/volume-modifier-for-k8s /bin/volume-modifier-for-k8s
ENTRYPOINT ["/bin/volume-modifier-for-k8s"]

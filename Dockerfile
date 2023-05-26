FROM --platform=$BUILDPLATFORM golang:1.20 AS builder
WORKDIR /go/src/github.com/kubernetes-sigs/aws-ebs-csi-driver
COPY go.* .
ARG GOPROXY=direct
RUN go mod download
COPY . .
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
RUN OS=$TARGETOS ARCH=$TARGETARCH make $TARGETOS/$TARGETARCH

FROM public.ecr.aws/eks-distro-build-tooling/eks-distro-minimal-base:latest.2 AS linux-amazon
COPY --from=builder /go/src/github.com/kubernetes-sigs/aws-ebs-csi-driver/bin/aws-ebs-csi-driver /bin/aws-ebs-csi-driver
ENTRYPOINT ["/bin/aws-ebs-csi-driver"]

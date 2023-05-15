FROM public.ecr.aws/eks-distro-build-tooling/eks-distro-minimal-base-csi-ebs:latest.2 AS linux-amazon
COPY ./bin/main /bin/volume-modifier-for-k8s
ENTRYPOINT ["/bin/volume-modifier-for-k8s"]

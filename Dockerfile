FROM public.ecr.aws/eks-distro-build-tooling/eks-distro-minimal-base:latest.2 AS linux-amazon

COPY ./main /bin/ebs-external-volume-modifier
ENTRYPOINT ["/bin/ebs-external-volume-modifier"]

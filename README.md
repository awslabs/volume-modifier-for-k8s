## volume-modifier-for-k8s

> [!CAUTION]
> This project has been deprecated in favor of the native Kubernetes [VolumeAttributesClass](https://kubernetes.io/docs/concepts/storage/volume-attributes-classes/) API.
> No new features will be added. Security and bug fixes will still be released until **April 1st, 2026**. See the [[Deprecation Announcement] volume-modifier-for-k8s](https://github.com/kubernetes-sigs/aws-ebs-csi-driver/issues/2435) for more information.

`volume-modifier-for-k8s` is a sidecar deployed alongside CSI drivers to enable volume modification through annotations on the PVC.

## Requirements

Leader election must be enabled in the [external-resizer](https://github.com/kubernetes-csi/external-resizer). This is required in order to efficiently coordinate calls to the EC2 modify-volume API.

## Security

See [CONTRIBUTING](CONTRIBUTING.md#security-issue-notifications) for more information.

## License

This project is licensed under the Apache-2.0 License.


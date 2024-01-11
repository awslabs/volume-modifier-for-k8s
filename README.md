## volume-modifier-for-k8s

`volume-modifier-for-k8s` is a sidecar deployed alongside CSI drivers to enable volume modification through annotations on the PVC.

## Requirements

Leader election must be enabled in the [external-resizer](https://github.com/kubernetes-csi/external-resizer). This is required in order to efficiently coordinate calls to the EC2 modify-volume API.

## Security

See [CONTRIBUTING](CONTRIBUTING.md#security-issue-notifications) for more information.

## License

This project is licensed under the Apache-2.0 License.


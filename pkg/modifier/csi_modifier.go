package modifier

import (
	"context"
	"fmt"
	"time"

	csi "github.com/awslabs/volume-modifier-for-k8s/pkg/client"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	csitrans "k8s.io/csi-translation-lib"
	"k8s.io/klog/v2"
)

func NewFromClient(
	name string,
	csiClient csi.Client,
	kubeClient kubernetes.Interface,
	timeout time.Duration,
) (Modifier, error) {
	return &csiModifier{
		name:      name,
		client:    csiClient,
		k8sClient: kubeClient,
		timeout:   timeout,
	}, nil
}

type csiModifier struct {
	name      string
	client    csi.Client
	timeout   time.Duration
	k8sClient kubernetes.Interface
}

func (c *csiModifier) Name() string {
	return c.name
}

func (c *csiModifier) Modify(pv *v1.PersistentVolume, params, reqContext map[string]string) error {
	klog.V(6).InfoS("Received modify request", "pv", pv, "params", params)

	var (
		volumeID string
	)

	if pv.Spec.CSI != nil {
		volumeID = pv.Spec.CSI.VolumeHandle
	} else {
		translator := csitrans.New()
		if translator.IsMigratedCSIDriverByName(c.name) {
			csiPV, err := translator.TranslateInTreePVToCSI(pv)
			if err != nil {
				return fmt.Errorf("failed to translate persistent volume: %w", err)
			}
			volumeID = csiPV.Spec.CSI.VolumeHandle
		} else {
			return fmt.Errorf("volume %v is not migrated to CSI", pv.Name)
		}
	}

	klog.InfoS("Calling modify volume for volume", "volumeID", volumeID)

	ctx, cancel := context.WithTimeout(context.TODO(), c.timeout)
	defer cancel()
	return c.client.Modify(ctx, volumeID, params, reqContext)
}

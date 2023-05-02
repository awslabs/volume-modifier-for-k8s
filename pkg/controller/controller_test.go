package controller

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	csi "github.com/awslabs/volume-modifier-for-k8s/pkg/client"
	"github.com/awslabs/volume-modifier-for-k8s/pkg/modifier"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"
)

const (
	namespace = "default"
)

func TestControllerRun(t *testing.T) {
	testCases := []struct {
		name                                   string
		driverName                             string
		clientReturnsError                     bool
		pvc                                    *v1.PersistentVolumeClaim
		pv                                     *v1.PersistentVolume
		additionalPVCAnnotations               map[string]string
		expectedModifyVolumeCallCount          int
		expectInProgressModificationAnnotation bool
		updatedCapacity                        string
		expectSuccessfulModification           bool
	}{
		{
			name:       "volume modification succeeds after updating annotation",
			driverName: "ebs.csi.aws.com",
			pvc:        newFakePVC(),
			pv:         newFakePV("testPVC", namespace, "test"),
			additionalPVCAnnotations: map[string]string{
				"ebs.csi.aws.com/volumeType": "io2",
				"ebs.csi.aws.com/iops":       "5000",
			},
			expectedModifyVolumeCallCount: 1,
			expectSuccessfulModification:  true,
		},
		{
			name:       "volume modification fails after client returns error",
			driverName: "ebs.csi.aws.com",
			pvc:        newFakePVC(),
			pv:         newFakePV("testPVC", namespace, "test"),
			additionalPVCAnnotations: map[string]string{
				"ebs.csi.aws.com/volumeType": "io2",
				"ebs.csi.aws.com/iops":       "5000",
			},
			expectedModifyVolumeCallCount: 1,
			clientReturnsError:            true,
			expectSuccessfulModification:  false,
		},
		{
			name:                          "no volume modification after PVC resync",
			driverName:                    "ebs.csi.aws.com",
			pvc:                           newFakePVC(),
			pv:                            newFakePV("testPVC", namespace, "test"),
			expectedModifyVolumeCallCount: 0,
			expectSuccessfulModification:  false,
		},
		{
			name:                          "no volume modification after non-annotation PVC update",
			driverName:                    "ebs.csi.aws.com",
			pvc:                           newFakePVC(),
			pv:                            newFakePV("testPVC", namespace, "test"),
			expectedModifyVolumeCallCount: 0,
			updatedCapacity:               "35Gi",
			expectSuccessfulModification:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := csi.NewFakeClient(tc.driverName, true, tc.clientReturnsError)
			driverName, _ := client.GetDriverName(context.TODO())

			var objects []runtime.Object
			if tc.pvc != nil {
				if tc.additionalPVCAnnotations != nil {
					for k, v := range tc.additionalPVCAnnotations {
						tc.pvc.Annotations[k] = v
					}
				}
				if tc.updatedCapacity != "" {
					capacity := resource.MustParse(tc.updatedCapacity)
					tc.pvc.Spec.Resources.Requests[v1.ResourceStorage] = capacity
				}
				objects = append(objects, tc.pvc)
			}
			if tc.pv != nil {
				tc.pv.Spec.PersistentVolumeSource.CSI.Driver = driverName
				objects = append(objects, tc.pv)
			}

			k8sClient := fake.NewSimpleClientset(objects...)
			factory := informers.NewSharedInformerFactory(k8sClient, 0)

			modifier, err := modifier.NewFromClient(tc.driverName, client, k8sClient, 0)
			if err != nil {
				t.Fatal(err)
			}

			controller := NewModifyController(
				tc.driverName,
				modifier,
				k8sClient,
				0,
				factory,
				workqueue.DefaultControllerRateLimiter(),
				false,
			)

			stopCh := make(chan struct{})
			factory.Start(stopCh)

			ctx := context.Background()
			defer ctx.Done()
			go controller.Run(1, ctx)

			time.Sleep(1 * time.Second)

			if client.GetModifyCallCount() != tc.expectedModifyVolumeCallCount {
				t.Fatalf("unexpected modify volume call count: expected %d, got %d", tc.expectedModifyVolumeCallCount, client.GetModifyCallCount())
			}

			updatedPV, err := k8sClient.CoreV1().PersistentVolumes().Get(context.TODO(), tc.pv.Name, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}

			if tc.expectSuccessfulModification {
				err = verifyAnnotationsOnPV(updatedPV.Annotations, tc.additionalPVCAnnotations)
			} else {
				err = verifyNoAnnotationsOnPV(updatedPV.Annotations, driverName)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func verifyNoAnnotationsOnPV(ann map[string]string, driverName string) error {
	for k, v := range ann {
		if strings.HasPrefix(k, driverName) {
			return fmt.Errorf("found annotation on PV: %s (value: %s)", k, v)
		}
	}
	return nil
}

func verifyAnnotationsOnPV(updatedAnnotations, expectedAnnotations map[string]string) error {
	for k, v := range expectedAnnotations {
		if updatedAnnotations[k] != v {
			return fmt.Errorf("unexpected annotation on PV: %s (value : %s)", k, v)
		}
	}
	return nil
}

func newFakePVC() *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testPVC",
			Namespace:   namespace,
			UID:         "test",
			Annotations: make(map[string]string),
		},
		Spec: v1.PersistentVolumeClaimSpec{
			Resources: v1.ResourceRequirements{
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceStorage: *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
				},
			},
			VolumeName: "testPV",
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: v1.ClaimBound,
			Capacity: map[v1.ResourceName]resource.Quantity{
				v1.ResourceStorage: *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
			},
		},
	}
}

func newFakePV(pvcName, pvcNamespace string, pvcUID types.UID) *v1.PersistentVolume {
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testPV",
			Annotations: make(map[string]string),
		},
		Spec: v1.PersistentVolumeSpec{
			Capacity: map[v1.ResourceName]resource.Quantity{
				v1.ResourceStorage: *resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CSI: &v1.CSIPersistentVolumeSource{
					Driver:       "test",
					VolumeHandle: "vol-123243434",
				},
			},
		},
	}
	if pvcName != "" {
		pv.Spec.ClaimRef = &v1.ObjectReference{
			Namespace: pvcNamespace,
			Name:      pvcName,
			UID:       pvcUID,
		}
		pv.Status.Phase = v1.VolumeBound
	}
	return pv
}

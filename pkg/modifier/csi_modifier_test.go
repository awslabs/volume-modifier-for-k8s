package modifier

import (
	"testing"

	csi "github.com/awslabs/volume-modifier-for-k8s/pkg/client"
	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewModifier(t *testing.T) {
	testCases := []struct {
		name       string
		driverName string
	}{
		{
			name:       "new modifier succeeds",
			driverName: "mock",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := csi.NewFakeClient(tc.name, true, false)
			k8sClient := getFakeKubernetesClient()
			modifier, err := NewFromClient(tc.driverName, client, k8sClient, 0)
			if err != nil {
				t.Fatal(err)
			}
			if modifier.Name() != tc.driverName {
				t.Fatalf("unexpected modifier name, expected %s, got %s", tc.driverName, modifier.Name())
			}
		})
	}
}

func TestModify(t *testing.T) {
	testCases := []struct {
		name               string
		driverName         string
		inTree             bool
		clientReturnsError bool
		params             map[string]string
		reqContext         map[string]string
	}{
		{
			name:       "modify succeeds",
			driverName: "ebs.csi.aws.com",
			params: map[string]string{
				"foo": "bar",
			},
			reqContext: map[string]string{
				"baz": "bar",
			},
		},
		{
			name:       "modify fails",
			driverName: "ebs.csi.aws.com",
			params: map[string]string{
				"foo": "bar",
			},
			reqContext: map[string]string{
				"baz": "bar",
			},
			clientReturnsError: true,
		},
		{
			name:       "test intree migrated PV modification succeeds",
			driverName: "ebs.csi.aws.com",
			inTree:     true,
			params: map[string]string{
				"foo": "bar",
			},
			reqContext: map[string]string{
				"baz": "bar",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := csi.NewFakeClient(tc.name, true, tc.clientReturnsError)
			k8sClient := getFakeKubernetesClient()
			volumeID := "vol-1234355446a2"
			var pv *v1.PersistentVolume
			if tc.inTree {
				pv = newFakeInTreePV("test", volumeID)
			} else {
				pv = newFakeCSIPV("test", tc.driverName, volumeID)
			}
			modifier, err := NewFromClient(tc.driverName, client, k8sClient, 0)
			if err != nil {
				t.Fatal(err)
			}
			err = modifier.Modify(pv, tc.params, tc.reqContext)
			if err != nil {
				if !tc.clientReturnsError {
					t.Fatal(err)
				}
			}
			if client.GetVolumeName() != volumeID {
				t.Fatalf("unexpected volume ID: got %s, expected %s", client.GetVolumeName(), volumeID)
			}
			if diff := cmp.Diff(client.GetParams(), tc.params); diff != "" {
				t.Fatalf("unexpected params: diff = %v", diff)
			}
			if diff := cmp.Diff(client.GetReqContext(), tc.reqContext); diff != "" {
				t.Fatalf("unexpected req context: diff = %v", diff)
			}
		})
	}
}

func newFakeInTreePV(name, volumeID string) *v1.PersistentVolume {
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.PersistentVolumeSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): resource.MustParse("4Gi"),
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				AWSElasticBlockStore: &v1.AWSElasticBlockStoreVolumeSource{
					VolumeID: volumeID,
					ReadOnly: false,
				},
			},
		},
	}
	return pv
}

func newFakeCSIPV(name, driverName, volumeID string) *v1.PersistentVolume {
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.PersistentVolumeSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): resource.MustParse("4Gi"),
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CSI: &v1.CSIPersistentVolumeSource{
					Driver:       driverName,
					VolumeHandle: volumeID,
					ReadOnly:     false,
				},
			},
		},
	}
	return pv
}

func getFakeKubernetesClient() kubernetes.Interface {
	return fake.NewSimpleClientset()
}

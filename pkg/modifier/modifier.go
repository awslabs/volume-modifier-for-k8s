package modifier

import v1 "k8s.io/api/core/v1"

type Modifier interface {
	Name() string

	Modify(*v1.PersistentVolume, map[string]string, map[string]string) error
}

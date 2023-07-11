package controller

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/awslabs/volume-modifier-for-k8s/pkg/modifier"
	"github.com/awslabs/volume-modifier-for-k8s/pkg/util"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/scheme"
)

type ModifyController interface {
	Run(int, context.Context)
}

func NewModifyController(
	name string,
	modifier modifier.Modifier,
	kubeClient kubernetes.Interface,
	resyncPeriod time.Duration,
	informerFactory informers.SharedInformerFactory,
	pvcRateLimiter workqueue.RateLimiter,
	retryModificationFailures bool,
) ModifyController {
	pvInformer := informerFactory.Core().V1().PersistentVolumes()
	pvcInformer := informerFactory.Core().V1().PersistentVolumeClaims()
	claimQueue := workqueue.NewNamedRateLimitingQueue(pvcRateLimiter, fmt.Sprintf("%s-modify-pvc", name))

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events(v1.NamespaceAll)})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: fmt.Sprintf("volume-modifier-for-k8s-%s", name)})

	ctrl := &modifyController{
		name:                   name,
		annPrefix:              fmt.Sprintf(AnnotationPrefixPattern, name),
		modifier:               modifier,
		kubeClient:             kubeClient,
		claimQueue:             claimQueue,
		pvSynced:               pvInformer.Informer().HasSynced,
		pvcSynced:              pvcInformer.Informer().HasSynced,
		volumes:                pvInformer.Informer().GetStore(),
		claims:                 pvcInformer.Informer().GetStore(),
		eventRecorder:          eventRecorder,
		modificationInProgress: make(map[string]struct{}),
	}

	pvcInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    ctrl.addPVC,
		UpdateFunc: ctrl.updatePVC,
		DeleteFunc: ctrl.deletePVC,
	}, resyncPeriod)

	return ctrl
}

type modifyController struct {
	name          string
	annPrefix     string
	modifier      modifier.Modifier
	kubeClient    kubernetes.Interface
	claimQueue    workqueue.RateLimitingInterface
	eventRecorder record.EventRecorder
	pvSynced      cache.InformerSynced
	pvcSynced     cache.InformerSynced

	modificationInProgress   map[string]struct{}
	modificationInProgressMu sync.Mutex

	volumes cache.Store
	claims  cache.Store

	retryFailures bool
}

func (c *modifyController) Run(workers int, ctx context.Context) {
	defer c.claimQueue.ShutDown()

	klog.InfoS("Starting external modifier", "name", c.name)
	defer klog.InfoS("Shutting down external modifier", "name", c.name)

	stopCh := ctx.Done()
	informersSyncd := []cache.InformerSynced{c.pvSynced, c.pvcSynced}

	if !cache.WaitForCacheSync(stopCh, informersSyncd...) {
		klog.Errorf("Cannot sync pv or pvc caches")
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(c.syncPVCs, 0, stopCh)
	}

	<-stopCh
}

func (c *modifyController) addPVC(obj interface{}) {
	objKey, err := getObjectKeys(obj)
	if err != nil {
		klog.ErrorS(err, "unable to add obj to claim queue")
		return
	}
	c.claimQueue.Add(objKey)
}

func (c *modifyController) updatePVC(old, new interface{}) {
	klog.V(6).InfoS("Received update from shared informer", "old", old, "new", new)

	oldPvc, ok := old.(*v1.PersistentVolumeClaim)
	if !ok || oldPvc == nil {
		return
	}

	newPvc, ok := new.(*v1.PersistentVolumeClaim)
	if !ok || newPvc == nil {
		return
	}

	if c.needsProcessing(oldPvc, newPvc) {
		c.addPVC(new)
	}
}

func (c *modifyController) deletePVC(obj interface{}) {
	klog.V(6).InfoS("Received delete from shared informer", "obj", obj)
	objKey, err := getObjectKeys(obj)
	if err != nil {
		return
	}
	c.claimQueue.Forget(objKey)
}

func (c *modifyController) syncPVCs() {
	key, quit := c.claimQueue.Get()
	if quit {
		return
	}
	defer c.claimQueue.Done(key)

	if err := c.syncPVC(key.(string)); err != nil {
		klog.ErrorS(err, "error syncing PVC", "key", key)
		if c.retryFailures {
			c.claimQueue.AddRateLimited(key)
		}
	} else {
		c.claimQueue.Forget(key)
	}
}

func (c *modifyController) syncPVC(key string) error {
	klog.InfoS("Started PVC processing", "key", key)
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("cannot get namespace and name from key (%s): %w", key, err)
	}

	pvcObject, exists, err := c.claims.GetByKey(key)
	if err != nil {
		return fmt.Errorf("cannot get PVC for key (%s): %w", key, err)
	}

	if !exists {
		klog.InfoS("PVC is deleted or does not exist", "namespace", namespace, "name", name)
		return nil
	}

	pvc, ok := pvcObject.(*v1.PersistentVolumeClaim)
	if !ok {
		return fmt.Errorf("expected PVC for key (%s) but got %v", key, pvcObject)
	}

	if pvc.Spec.VolumeName == "" {
		klog.InfoS("PV bound to PVC is not created yet", "pvc", util.PVCKey(pvc))
		return nil
	}

	volumeObj, exists, err := c.volumes.GetByKey(pvc.Spec.VolumeName)
	if err != nil {
		return fmt.Errorf("get PV %q of pvc %q failed: %v", pvc.Spec.VolumeName, util.PVCKey(pvc), err)
	}

	if !exists {
		klog.Warningf("PV %q bound to PVC %s not found", pvc.Spec.VolumeName, util.PVCKey(pvc))
		return nil
	}

	pv, ok := volumeObj.(*v1.PersistentVolume)
	if !ok {
		return fmt.Errorf("expected volume but got %+v", volumeObj)
	}

	if !c.pvcNeedsModification(pv, pvc) {
		klog.InfoS("No need to modify PVC", "pvc", util.PVCKey(pvc))
		return nil
	}

	return c.modifyPVC(pv, pvc)
}

// Determines if the PVC needs modification.
func (c *modifyController) pvcNeedsModification(pv *v1.PersistentVolume, pvc *v1.PersistentVolumeClaim) bool {
	// Check if there's already a modification going on.
	if c.ifPVCModificationInProgress(pvc.Name) {
		klog.InfoS("modification for pvc is already undergoing", "pvc", util.PVCKey(pvc))
		return false
	}

	// Only Bound PVC can be modified
	if pvc.Status.Phase != v1.ClaimBound {
		klog.InfoS("pvc is not bound", "pvc", util.PVCKey(pvc))
		return false
	}

	if pvc.Spec.VolumeName == "" {
		klog.InfoS("volume name is empty", "pvc", util.PVCKey(pvc))
		return false
	}

	if !c.annotationsUpdated(pvc.Annotations, pv.Annotations) {
		klog.InfoS("annotations not updated", "pvc", util.PVCKey(pvc))
		return false
	}

	return true
}

func (c *modifyController) addPVCToInProgressList(pvc string) {
	c.modificationInProgressMu.Lock()
	defer c.modificationInProgressMu.Unlock()
	c.modificationInProgress[pvc] = struct{}{}
}

func (c *modifyController) ifPVCModificationInProgress(pvc string) bool {
	c.modificationInProgressMu.Lock()
	defer c.modificationInProgressMu.Unlock()
	_, ok := c.modificationInProgress[pvc]
	return ok
}

func (c *modifyController) removePVCFromInProgressList(pvc string) {
	c.modificationInProgressMu.Lock()
	defer c.modificationInProgressMu.Unlock()
	delete(c.modificationInProgress, pvc)
}

func (c *modifyController) modifyPVC(pv *v1.PersistentVolume, pvc *v1.PersistentVolumeClaim) error {
	c.addPVCToInProgressList(pvc.Name)
	defer c.removePVCFromInProgressList(pvc.Name)

	params := make(map[string]string)
	for key, value := range pvc.Annotations {
		if c.isValidAnnotation(key) {
			params[c.attributeFromValidAnnotation(key)] = value
		}
	}

	reqContext := make(map[string]string)

	c.eventRecorder.Event(pvc, v1.EventTypeNormal, VolumeModificationStarted, fmt.Sprintf("External modifier is modifying volume %s", pv.Name))

	err := c.modifier.Modify(pv, params, reqContext)
	if err != nil {
		c.eventRecorder.Eventf(pvc, v1.EventTypeWarning, VolumeModificationFailed, err.Error())
		return fmt.Errorf("modification of volume %q failed by modifier %q: %w", pvc.Name, c.name, err)
	} else {
		c.eventRecorder.Eventf(pvc, v1.EventTypeNormal, VolumeModificationSuccessful, "External modifier has successfully modified volume %s", pv.Name)
	}

	return c.markPVCModificationComplete(pv, params)
}

func (c *modifyController) isValidAnnotation(ann string) bool {
	return strings.HasPrefix(ann, fmt.Sprintf(AnnotationPrefixPattern, c.name)) &&
		!strings.HasSuffix(ann, "-status")
}

func (c *modifyController) attributeFromValidAnnotation(ann string) string {
	return strings.TrimPrefix(ann, fmt.Sprintf(AnnotationPrefixPattern, c.name))
}

func (c *modifyController) markPVCModificationComplete(oldPV *v1.PersistentVolume, params map[string]string) error {
	newPV := oldPV.DeepCopy()
	for key, value := range params {
		newPV.Annotations[fmt.Sprintf("%s/%s", c.name, key)] = value
	}

	_, err := c.patchPV(oldPV, newPV, true)
	return err
}

func (c *modifyController) patchPV(old, new *v1.PersistentVolume, addResourceVersionCheck bool) (*v1.PersistentVolume, error) {
	patchBytes, err := util.GetPatchData(old, new)
	if err != nil {
		return old, fmt.Errorf("can't patch status of PV %s as patch data generation failed: %v", old.Name, err)
	}

	updatedPV, err := c.kubeClient.CoreV1().PersistentVolumes().
		Patch(context.TODO(), old.Name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})

	if err != nil {
		return old, fmt.Errorf("can't patch PV %s with %v", old.Name, err)
	}

	err = c.volumes.Update(updatedPV)
	if err != nil {
		return old, fmt.Errorf("error updating PV %s in local cache: %v", old.Name, err)
	}
	return updatedPV, nil
}

// Check if annotations are updated.
func (c *modifyController) annotationsUpdated(pvcAnnotations, pvAnnotations map[string]string) bool {
	m := make(map[string]string)
	for key, value := range pvcAnnotations {
		if c.isValidAnnotation(key) {
			m[key] = value
		}
	}

	for key, value := range m {
		if pvAnnotations[key] != value {
			return true
		}
	}

	return false
}

// Checks if a PVC needs to be processed after an Update.
// Gets a list of all annotations beginning with "<driver-name>/" both PVCs.
// Then checks if the annotations are different between the old and new PVCs.
// If any of them are, this PVC needs to be processed.
func (c *modifyController) needsProcessing(old *v1.PersistentVolumeClaim, new *v1.PersistentVolumeClaim) bool {
	if old.ResourceVersion == new.ResourceVersion {
		return false
	}

	annotations := make(map[string]struct{})
	for key, _ := range new.Annotations {
		if c.isValidAnnotation(key) {
			annotations[key] = struct{}{}
		}
	}

	for a, _ := range annotations {
		oldValue := old.Annotations[a]
		newValue := new.Annotations[a]

		if oldValue != newValue {
			return true
		}
	}

	hasBeenBound := old.Status.Phase != new.Status.Phase && new.Status.Phase == v1.ClaimBound
	// If the annotation was set at creation we might have skipped the PVC because it was not bound yet
	return len(annotations) > 0 && hasBeenBound
}

func getObjectKeys(obj interface{}) (string, error) {
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}

	objKey, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		klog.Errorf("Failed to get key from object: %v", err)
		return "", err
	}
	return objKey, nil
}

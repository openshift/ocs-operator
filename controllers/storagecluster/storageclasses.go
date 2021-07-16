package storagecluster

import (
	"context"
	"fmt"
	"reflect"

	ocsv1 "github.com/openshift/ocs-operator/api/v1"
	"github.com/openshift/ocs-operator/controllers/util"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

// StorageClassConfiguration provides configuration options for a StorageClass.
type StorageClassConfiguration struct {
	storageClass      *storagev1.StorageClass
	reconcileStrategy ReconcileStrategy
	disable           bool
}

type ocsStorageClass struct{}

// ensureCreated ensures that StorageClass resources exist in the desired
// state.
func (obj *ocsStorageClass) ensureCreated(r *StorageClusterReconciler, instance *ocsv1.StorageCluster) error {
	scs, err := r.newStorageClassConfigurations(instance)
	if err != nil {
		return err
	}

	if !instance.Spec.ManagedResources.CephBlockPools.DisableStorageClass {
		// wait for CephBlockPool to be ready
		cephBlockPool := cephv1.CephBlockPool{}
		key := types.NamespacedName{Name: generateNameForCephBlockPool(instance), Namespace: instance.Namespace}
		r.Log.Info("Waiting for CephBlockPool %q to be ready before creating storage class.", key)
		err = r.Client.Get(context.TODO(), key, &cephBlockPool)
		if err != nil {
			r.Log.Info("Error while waiting for CephBlockPool %q.", key, "Error:", err)
			return err
		}
		if cephBlockPool.Status == nil {
			return fmt.Errorf("cephBlockPool %q is not reporting status", key)
		}
		r.Log.Info("CephBlockPool %q is in phase %q", key, cephBlockPool.Status.Phase)
		if cephBlockPool.Status.Phase != cephv1.ConditionType(util.PhaseReady) {
			return fmt.Errorf("cephBlockPool %q is not %q", key, util.PhaseReady)
		}
	}

	if !instance.Spec.ManagedResources.CephFilesystems.DisableStorageClass {
		// wait for CephFilesystem to be ready
		cephFilesystem := cephv1.CephFilesystem{}
		key := types.NamespacedName{Name: generateNameForCephFilesystem(instance), Namespace: instance.Namespace}
		r.Log.Info("Waiting for CephFilesystem %q to be ready before creating storage class.", key)
		err = r.Client.Get(context.TODO(), key, &cephFilesystem)
		if err != nil {
			r.Log.Info("Error while waiting for CephFilesystem %q.", key, "Error:", err)
			return err
		}
		if cephFilesystem.Status == nil {
			return fmt.Errorf("cephFilesystem %q is not reporting status", key)
		}
		r.Log.Info("CephFilesystem %q is in phase %q", key, cephFilesystem.Status.Phase)
		if cephFilesystem.Status.Phase != util.PhaseReady {
			return fmt.Errorf("cephFilesystem %q is not %q", key, util.PhaseReady)
		}
	}

	err = r.createStorageClasses(scs)
	if err != nil {
		return err
	}

	return nil
}

// ensureDeleted deletes the storageClasses that the ocs-operator created
func (obj *ocsStorageClass) ensureDeleted(r *StorageClusterReconciler, instance *ocsv1.StorageCluster) error {

	sccs, err := r.newStorageClassConfigurations(instance)
	if err != nil {
		r.Log.Error(err, "Uninstall: Unable to determine the StorageClass names.") //nolint:gosimple
		return nil
	}
	for _, scc := range sccs {
		sc := scc.storageClass
		existing := storagev1.StorageClass{}
		err := r.Client.Get(context.TODO(), types.NamespacedName{Name: sc.Name, Namespace: sc.Namespace}, &existing)

		switch {
		case err == nil:
			if existing.DeletionTimestamp != nil {
				r.Log.Info("Uninstall: StorageClass is already marked for deletion.", "StorageClass", klog.KRef(sc.Namespace, existing.Name))
				break
			}

			r.Log.Info("Uninstall: Deleting StorageClass.", "StorageClass", klog.KRef(sc.Namespace, existing.Name))
			existing.ObjectMeta.OwnerReferences = sc.ObjectMeta.OwnerReferences
			sc.ObjectMeta = existing.ObjectMeta

			err = r.Client.Delete(context.TODO(), sc)
			if err != nil {
				r.Log.Error(err, "Uninstall: Ignoring error deleting the StorageClass.", "StorageClass", klog.KRef(sc.Namespace, existing.Name))
			}
		case errors.IsNotFound(err):
			r.Log.Info("Uninstall: StorageClass not found, nothing to do.", "StorageClass", klog.KRef(sc.Namespace, existing.Name))
		default:
			r.Log.Error(err, "Uninstall: Error while getting StorageClass.", "StorageClass", klog.KRef(sc.Namespace, existing.Name))
		}
	}
	return nil
}

func (r *StorageClusterReconciler) createStorageClasses(sccs []StorageClassConfiguration) error {
	for _, scc := range sccs {
		if scc.reconcileStrategy == ReconcileStrategyIgnore || scc.disable {
			continue
		}
		sc := scc.storageClass
		existing := &storagev1.StorageClass{}
		err := r.Client.Get(context.TODO(), types.NamespacedName{Name: sc.Name, Namespace: sc.Namespace}, existing)

		if errors.IsNotFound(err) {
			// Since the StorageClass is not found, we will create a new one
			r.Log.Info("Creating StorageClass.", "StorageClass", klog.KRef(sc.Namespace, existing.Name))
			err = r.Client.Create(context.TODO(), sc)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			if scc.reconcileStrategy == ReconcileStrategyInit {
				continue
			}
			if existing.DeletionTimestamp != nil {
				return fmt.Errorf("failed to restore StorageClass  %s because it is marked for deletion", existing.Name)
			}
			if !reflect.DeepEqual(sc.Parameters, existing.Parameters) {
				// Since we have to update the existing StorageClass
				// So, we will delete the existing storageclass and create a new one
				r.Log.Info("StorageClass needs to be updated, deleting it.", "StorageClass", klog.KRef(sc.Namespace, existing.Name))
				err = r.Client.Delete(context.TODO(), existing)
				if err != nil {
					r.Log.Error(err, "Failed to delete StorageClass.", "StorageClass", klog.KRef(sc.Namespace, existing.Name))
					return err
				}
				r.Log.Info("Creating StorageClass.", "StorageClass", klog.KRef(sc.Namespace, sc.Name))
				err = r.Client.Create(context.TODO(), sc)
				if err != nil {
					r.Log.Info("Failed to craete StorageClass.", "StorageClass", klog.KRef(sc.Namespace, sc.Name))
					return err
				}
			}
		}
	}
	return nil
}

// newCephFilesystemStorageClassConfiguration generates configuration options for a Ceph Filesystem StorageClass.
func newCephFilesystemStorageClassConfiguration(initData *ocsv1.StorageCluster) StorageClassConfiguration {
	persistentVolumeReclaimDelete := corev1.PersistentVolumeReclaimDelete
	allowVolumeExpansion := true
	managementSpec := initData.Spec.ManagedResources.CephFilesystems
	return StorageClassConfiguration{
		storageClass: &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: generateNameForCephFilesystemSC(initData),
				Annotations: map[string]string{
					"description": "Provides RWO and RWX Filesystem volumes",
				},
			},
			Provisioner:   fmt.Sprintf("%s.cephfs.csi.ceph.com", initData.Namespace),
			ReclaimPolicy: &persistentVolumeReclaimDelete,
			// AllowVolumeExpansion is set to true to enable expansion of OCS backed Volumes
			AllowVolumeExpansion: &allowVolumeExpansion,
			Parameters: map[string]string{
				"clusterID": initData.Namespace,
				"fsName":    fmt.Sprintf("%s-cephfilesystem", initData.Name),
				"csi.storage.k8s.io/provisioner-secret-name":            "rook-csi-cephfs-provisioner",
				"csi.storage.k8s.io/provisioner-secret-namespace":       initData.Namespace,
				"csi.storage.k8s.io/node-stage-secret-name":             "rook-csi-cephfs-node",
				"csi.storage.k8s.io/node-stage-secret-namespace":        initData.Namespace,
				"csi.storage.k8s.io/controller-expand-secret-name":      "rook-csi-cephfs-provisioner",
				"csi.storage.k8s.io/controller-expand-secret-namespace": initData.Namespace,
			},
		},
		reconcileStrategy: ReconcileStrategy(managementSpec.ReconcileStrategy),
		disable:           managementSpec.DisableStorageClass,
	}
}

// newCephBlockPoolStorageClassConfiguration generates configuration options for a Ceph Block Pool StorageClass.
func newCephBlockPoolStorageClassConfiguration(initData *ocsv1.StorageCluster, thickProvision bool) StorageClassConfiguration {
	thickProvisionStr := "false"
	storageClassSuffix := ""
	if thickProvision {
		thickProvisionStr = "true"
		storageClassSuffix = "-thick"
	}

	persistentVolumeReclaimDelete := corev1.PersistentVolumeReclaimDelete
	allowVolumeExpansion := true
	managementSpec := initData.Spec.ManagedResources.CephBlockPools
	return StorageClassConfiguration{
		storageClass: &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: generateNameForCephBlockPoolSC(initData, storageClassSuffix),
				Annotations: map[string]string{
					"description": "Provides RWO Filesystem volumes, and RWO and RWX Block volumes",
				},
			},
			Provisioner:   fmt.Sprintf("%s.rbd.csi.ceph.com", initData.Namespace),
			ReclaimPolicy: &persistentVolumeReclaimDelete,
			// AllowVolumeExpansion is set to true to enable expansion of OCS backed Volumes
			AllowVolumeExpansion: &allowVolumeExpansion,
			Parameters: map[string]string{
				"clusterID":                 initData.Namespace,
				"pool":                      generateNameForCephBlockPool(initData),
				"imageFeatures":             "layering",
				"csi.storage.k8s.io/fstype": "ext4",
				"imageFormat":               "2",
				"thickProvision":            thickProvisionStr,
				"csi.storage.k8s.io/provisioner-secret-name":            "rook-csi-rbd-provisioner",
				"csi.storage.k8s.io/provisioner-secret-namespace":       initData.Namespace,
				"csi.storage.k8s.io/node-stage-secret-name":             "rook-csi-rbd-node",
				"csi.storage.k8s.io/node-stage-secret-namespace":        initData.Namespace,
				"csi.storage.k8s.io/controller-expand-secret-name":      "rook-csi-rbd-provisioner",
				"csi.storage.k8s.io/controller-expand-secret-namespace": initData.Namespace,
			},
		},
		reconcileStrategy: ReconcileStrategy(managementSpec.ReconcileStrategy),
		disable:           managementSpec.DisableStorageClass,
	}
}

// newCephOBCStorageClassConfiguration generates configuration options for a Ceph Object Store StorageClass.
func newCephOBCStorageClassConfiguration(initData *ocsv1.StorageCluster) StorageClassConfiguration {
	reclaimPolicy := corev1.PersistentVolumeReclaimDelete
	managementSpec := initData.Spec.ManagedResources.CephObjectStores
	return StorageClassConfiguration{
		storageClass: &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: generateNameForCephRgwSC(initData),
				Annotations: map[string]string{
					"description": "Provides Object Bucket Claims (OBCs)",
				},
			},
			Provisioner:   fmt.Sprintf("%s.ceph.rook.io/bucket", initData.Namespace),
			ReclaimPolicy: &reclaimPolicy,
			Parameters: map[string]string{
				"objectStoreNamespace": initData.Namespace,
				"region":               "us-east-1",
				"objectStoreName":      generateNameForCephObjectStore(initData),
			},
		},
		reconcileStrategy: ReconcileStrategy(managementSpec.ReconcileStrategy),
		disable:           managementSpec.DisableStorageClass,
	}
}

// newStorageClassConfigurations returns the StorageClassConfiguration instances that should be created
// on first run.
func (r *StorageClusterReconciler) newStorageClassConfigurations(initData *ocsv1.StorageCluster) ([]StorageClassConfiguration, error) {
	ret := []StorageClassConfiguration{
		newCephFilesystemStorageClassConfiguration(initData),
		newCephBlockPoolStorageClassConfiguration(initData, false),
		newCephBlockPoolStorageClassConfiguration(initData, true),
	}
	// OBC storageclass will be returned only in TWO conditions,
	// a. either 'externalStorage' is enabled
	// OR
	// b. current platform is not a cloud-based platform
	avoid, err := r.PlatformsShouldAvoidObjectStore()
	if initData.Spec.ExternalStorage.Enable || err == nil && !avoid {
		ret = append(ret, newCephOBCStorageClassConfiguration(initData))
	}
	return ret, nil
}

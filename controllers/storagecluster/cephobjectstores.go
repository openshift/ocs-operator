package storagecluster

import (
	"context"
	"fmt"

	ocsv1 "github.com/openshift/ocs-operator/api/v1"
	"github.com/openshift/ocs-operator/controllers/defaults"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ensureCephObjectStores ensures that CephObjectStore resources exist in the desired
// state.
func (r *StorageClusterReconciler) ensureCephObjectStores(instance *ocsv1.StorageCluster) error {
	reconcileStrategy := ReconcileStrategy(instance.Spec.ManagedResources.CephObjectStores.ReconcileStrategy)
	if reconcileStrategy == ReconcileStrategyIgnore {
		return nil
	}
	platform, err := r.Platform.GetPlatform(r.Client)
	if err != nil {
		return err
	}
	if avoidObjectStore(platform) {
		r.Log.Info(fmt.Sprintf("not creating a CephObjectStore because the platform is '%s'", platform))
		return nil
	}

	cephObjectStores, err := r.newCephObjectStoreInstances(instance)
	if err != nil {
		return err
	}
	err = r.createCephObjectStores(cephObjectStores, instance)
	if err != nil {
		r.Log.Error(err, "could not create CephObjectStores")
		return err
	}

	return nil
}

// createCephObjectStore creates CephObjectStore in the desired state
func (r *StorageClusterReconciler) createCephObjectStores(cephObjectStores []*cephv1.CephObjectStore, instance *ocsv1.StorageCluster) error {
	for _, cephObjectStore := range cephObjectStores {
		existing := cephv1.CephObjectStore{}
		err := r.Client.Get(context.TODO(), types.NamespacedName{Name: cephObjectStore.Name, Namespace: cephObjectStore.Namespace}, &existing)
		switch {
		case err == nil:
			reconcileStrategy := ReconcileStrategy(instance.Spec.ManagedResources.CephObjectStores.ReconcileStrategy)
			if reconcileStrategy == ReconcileStrategyInit {
				return nil
			}
			if existing.DeletionTimestamp != nil {
				err := fmt.Errorf("failed to restore cephobjectstore object %s because it is marked for deletion", existing.Name)
				r.Log.Info("cephobjectstore restore failed")
				return err
			}

			r.Log.Info(fmt.Sprintf("Restoring original cephObjectStore %s", cephObjectStore.Name))
			existing.ObjectMeta.OwnerReferences = cephObjectStore.ObjectMeta.OwnerReferences
			cephObjectStore.ObjectMeta = existing.ObjectMeta
			err = r.Client.Update(context.TODO(), cephObjectStore)
			if err != nil {
				r.Log.Error(err, fmt.Sprintf("failed to update CephObjectStore Object: %s", cephObjectStore.Name))
				return err
			}
		case errors.IsNotFound(err):
			r.Log.Info(fmt.Sprintf("creating CephObjectStore %s", cephObjectStore.Name))
			err = r.Client.Create(context.TODO(), cephObjectStore)
			if err != nil {
				r.Log.Error(err, fmt.Sprintf("failed to create CephObjectStore object: %s", cephObjectStore.Name))
				return err
			}
		}
	}
	return nil
}

// newCephObjectStoreInstances returns the cephObjectStore instances that should be created
// on first run.
func (r *StorageClusterReconciler) newCephObjectStoreInstances(initData *ocsv1.StorageCluster) ([]*cephv1.CephObjectStore, error) {
	ret := []*cephv1.CephObjectStore{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      generateNameForCephObjectStore(initData),
				Namespace: initData.Namespace,
			},
			Spec: cephv1.ObjectStoreSpec{
				PreservePoolsOnDelete: false,
				DataPool: cephv1.PoolSpec{
					FailureDomain: initData.Status.FailureDomain,
					Replicated: cephv1.ReplicatedSpec{
						Size:            3,
						TargetSizeRatio: .49,
					},
				},
				MetadataPool: cephv1.PoolSpec{
					FailureDomain: initData.Status.FailureDomain,
					Replicated: cephv1.ReplicatedSpec{
						Size: 3,
					},
				},
				Gateway: cephv1.GatewaySpec{
					Port:      80,
					Instances: 2,
					Placement: getPlacement(initData, "rgw"),
					Resources: defaults.GetDaemonResources("rgw", initData.Spec.Resources),
				},
			},
		},
	}
	for _, obj := range ret {
		err := controllerutil.SetControllerReference(initData, obj, r.Scheme)
		if err != nil {
			r.Log.Error(err, fmt.Sprintf("Failed to set ControllerReference to %s", obj.Name))
			return nil, err
		}
	}
	return ret, nil
}

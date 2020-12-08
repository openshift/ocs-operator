package storagecluster

import (
	"context"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	api "github.com/openshift/ocs-operator/pkg/apis/ocs/v1"
	"github.com/stretchr/testify/assert"
	storagev1 "k8s.io/api/storage/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var allPlatforms = append(AvoidObjectStorePlatforms,
	configv1.NonePlatformType, configv1.PlatformType("NonCloudPlatform"))

func TestStorageClasses(t *testing.T) {
	for _, eachPlatform := range allPlatforms {
		cp := &Platform{platform: eachPlatform}
		t, reconciler, cr, request := initStorageClusterResourceCreateUpdateTestWithPlatform(
			t, cp, nil)
		assertStorageClasses(t, reconciler, cr, request)
	}

}

func assertStorageClasses(t *testing.T, reconciler ReconcileStorageCluster, cr *api.StorageCluster, request reconcile.Request) {
	actualSc1 := &storagev1.StorageClass{}
	actualSc2 := &storagev1.StorageClass{}
	actualSc3 := &storagev1.StorageClass{}

	request.Name = "ocsinit-cephfs"
	err := reconciler.client.Get(context.TODO(), request.NamespacedName, actualSc1)
	assert.NoError(t, err)

	request.Name = "ocsinit-ceph-rbd"
	err = reconciler.client.Get(context.TODO(), request.NamespacedName, actualSc2)
	assert.NoError(t, err)

	expected, err := reconciler.newStorageClasses(cr)
	assert.NoError(t, err)
	request.Name = "ocsinit-ceph-rgw"
	err = reconciler.client.Get(context.TODO(), request.NamespacedName, actualSc3)
	// on a cloud platform, 'Get' should throw an error,
	// as OBC StorageClass won't be created
	if avoidObjectStore(reconciler.platform.platform) {
		// we should be expecting only 2 storage classes
		assert.Equal(t, len(expected), 2)
		assert.Error(t, err)
	} else {
		// if not a cloud platform, OBC Storage class should be created/updated
		assert.Equal(t, len(expected), 3)
		assert.NoError(t, err)
		assert.Equal(t, len(expected[2].OwnerReferences), 0)
		assert.Equal(t, expected[2].ObjectMeta.Name, actualSc3.ObjectMeta.Name)
		assert.Equal(t, expected[2].Provisioner, actualSc3.Provisioner)
		assert.Equal(t, expected[2].ReclaimPolicy, actualSc3.ReclaimPolicy)
		assert.Equal(t, expected[2].Parameters, actualSc3.Parameters)
		// Doing a bit more validation for the RGW SC since some fields differ whether
		// we do independent or converged mode, typically "objectStoreName" param must exist
		assert.NotEmpty(t, actualSc3.Parameters["objectStoreName"], actualSc3.Parameters)
		assert.NotEmpty(t, actualSc3.Parameters["region"], actualSc3.Parameters)
		assert.Equal(t, 3, len(actualSc3.Parameters))
	}

	// The created StorageClasses should not have any ownerReferences set. Any
	// OwnerReference set will be a cross-namespace OwnerReference, which could
	// lead to other child resources getting GCd.
	// Ref: https://bugzilla.redhat.com/show_bug.cgi?id=1755623
	// Ref: https://bugzilla.redhat.com/show_bug.cgi?id=1691546
	assert.Equal(t, len(expected[0].OwnerReferences), 0)
	assert.Equal(t, len(expected[1].OwnerReferences), 0)

	assert.Equal(t, expected[0].ObjectMeta.Name, actualSc1.ObjectMeta.Name)
	assert.Equal(t, expected[0].Provisioner, actualSc1.Provisioner)
	assert.Equal(t, expected[0].ReclaimPolicy, actualSc1.ReclaimPolicy)
	assert.Equal(t, expected[0].Parameters, actualSc1.Parameters)

	assert.Equal(t, expected[1].ObjectMeta.Name, actualSc2.ObjectMeta.Name)
	assert.Equal(t, expected[1].Provisioner, actualSc2.Provisioner)
	assert.Equal(t, expected[1].ReclaimPolicy, actualSc2.ReclaimPolicy)
	assert.Equal(t, expected[1].Parameters, actualSc2.Parameters)
}

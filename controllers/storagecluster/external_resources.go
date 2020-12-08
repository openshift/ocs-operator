package storagecluster

import (
	"context"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	ocsv1 "github.com/openshift/ocs-operator/pkg/apis/ocs/v1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	externalClusterDetailsSecret = "rook-ceph-external-cluster-details"
	externalClusterDetailsKey    = "external_cluster_details"
	cephFsStorageClassName       = "cephfs"
	cephRbdStorageClassName      = "ceph-rbd"
	cephRgwStorageClassName      = "ceph-rgw"
	externalCephRgwEndpointKey   = "endpoint"
)

const (
	rookCephOperatorConfigName = "rook-ceph-operator-config"
	rookEnableCephFSCSIKey     = "ROOK_CSI_ENABLE_CEPHFS"
)

// ExternalResource containes a list of External Cluster Resources
type ExternalResource struct {
	Kind string            `json:"kind"`
	Data map[string]string `json:"data"`
	Name string            `json:"name"`
}

// setRookCSICephFS function enables or disables the 'ROOK_CSI_ENABLE_CEPHFS' key
func (r *ReconcileStorageCluster) setRookCSICephFS(
	enableDisableFlag bool, instance *ocsv1.StorageCluster, reqLogger logr.Logger) error {
	rookCephOperatorConfig := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{Name: rookCephOperatorConfigName, Namespace: instance.ObjectMeta.Namespace},
		rookCephOperatorConfig)
	if err != nil {
		reqLogger.Error(err, fmt.Sprintf("Unable to get '%s' config", rookCephOperatorConfigName))
		return err
	}
	enableDisableFlagStr := fmt.Sprintf("%v", enableDisableFlag)
	// if the current state of 'ROOK_CSI_ENABLE_CEPHFS' flag is same, just return
	if rookCephOperatorConfig.Data[rookEnableCephFSCSIKey] == enableDisableFlagStr {
		return nil
	}
	rookCephOperatorConfig.Data[rookEnableCephFSCSIKey] = enableDisableFlagStr
	return r.client.Update(context.TODO(), rookCephOperatorConfig)
}

func checkRGWEndpoint(endpoint string, timeout time.Duration) error {
	con, err := net.DialTimeout("tcp", endpoint, timeout)
	if err != nil {
		return err
	}
	defer con.Close()
	return nil
}

func sha512sum(tobeHashed []byte) (string, error) {
	h := sha512.New()
	if _, err := h.Write(tobeHashed); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (r *ReconcileStorageCluster) externalSecretDataChecksum(instance *ocsv1.StorageCluster) (string, error) {
	found, err := r.retrieveSecret(externalClusterDetailsSecret, instance)
	if err != nil {
		return "", err
	}
	return sha512sum(found.Data[externalClusterDetailsKey])
}

func (r *ReconcileStorageCluster) sameExternalSecretData(instance *ocsv1.StorageCluster) bool {
	extSecretChecksum, err := r.externalSecretDataChecksum(instance)
	if err != nil {
		return false
	}
	// if the 'ExternalSecretHash' and fetched hash are same, then return true
	if instance.Status.ExternalSecretHash == extSecretChecksum {
		return true
	}
	// at this point the checksums are different, so update it
	instance.Status.ExternalSecretHash = extSecretChecksum
	return false
}

// retrieveSecret function retrieves the secret object with the specified name
func (r *ReconcileStorageCluster) retrieveSecret(secretName string, instance *ocsv1.StorageCluster) (*corev1.Secret, error) {
	found := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: instance.Namespace,
		},
	}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: found.Name, Namespace: found.Namespace}, found)
	return found, err
}

// retrieveExternalSecretData function retrieves the external secret and returns the data it contains
func (r *ReconcileStorageCluster) retrieveExternalSecretData(
	instance *ocsv1.StorageCluster, reqLogger logr.Logger) ([]ExternalResource, error) {
	found, err := r.retrieveSecret(externalClusterDetailsSecret, instance)
	if err != nil {
		reqLogger.Error(err, "could not find the external secret resource")
		return nil, err
	}
	var data []ExternalResource
	err = json.Unmarshal(found.Data[externalClusterDetailsKey], &data)
	if err != nil {
		reqLogger.Error(err, "could not parse json blob")
		return nil, err
	}
	return data, nil
}

func newExternalGatewaySpec(rgwEndpoint string, reqLogger logr.Logger) (*cephv1.GatewaySpec, error) {
	var gateWay cephv1.GatewaySpec
	hostIP, portStr, err := net.SplitHostPort(rgwEndpoint)
	if err != nil {
		reqLogger.Error(err,
			fmt.Sprintf("invalid rgw endpoint provided: %s", rgwEndpoint))
		return nil, err
	}
	if hostIP == "" {
		err := fmt.Errorf("An empty rgw host 'IP' address found")
		reqLogger.Error(err, "Host IP should not be empty in rgw endpoint")
		return nil, err
	}
	gateWay.ExternalRgwEndpoints = []corev1.EndpointAddress{{IP: hostIP}}
	var portInt64 int64
	if portInt64, err = strconv.ParseInt(portStr, 10, 32); err != nil {
		reqLogger.Error(err,
			fmt.Sprintf("invalid rgw 'port' provided: %s", portStr))
		return nil, err
	}
	gateWay.Port = int32(portInt64)
	return &gateWay, nil
}

// newExternalCephObjectStoreInstances returns a set of CephObjectStores
// needed for external cluster mode
func (r *ReconcileStorageCluster) newExternalCephObjectStoreInstances(
	initData *ocsv1.StorageCluster, rgwEndpoint string, reqLogger logr.Logger) ([]*cephv1.CephObjectStore, error) {
	// check whether the provided rgw endpoint is empty
	if rgwEndpoint = strings.TrimSpace(rgwEndpoint); rgwEndpoint == "" {
		reqLogger.Info("WARNING: Empty RGW Endpoint specified, external CephObjectStore won't be created")
		return nil, nil
	}
	gatewaySpec, err := newExternalGatewaySpec(rgwEndpoint, reqLogger)
	if err != nil {
		return nil, err
	}
	// enable bucket healthcheck
	healthCheck := cephv1.BucketHealthCheckSpec{
		Bucket: cephv1.HealthCheckSpec{
			Disabled: false,
			Interval: "60s",
		},
	}
	retObj := &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateNameForCephObjectStore(initData),
			Namespace: initData.Namespace,
		},
		Spec: cephv1.ObjectStoreSpec{
			Gateway:     *gatewaySpec,
			HealthCheck: healthCheck,
		},
	}
	retArrObj := []*cephv1.CephObjectStore{
		retObj,
	}
	return retArrObj, nil
}

// ensureExternalStorageClusterResources ensures that requested resources for the external cluster
// being created
func (r *ReconcileStorageCluster) ensureExternalStorageClusterResources(instance *ocsv1.StorageCluster, reqLogger logr.Logger) error {
	if r.sameExternalSecretData(instance) {
		return nil
	}
	err := r.createExternalStorageClusterResources(instance, reqLogger)
	if err != nil {
		reqLogger.Error(err, "could not create ExternalStorageClusterResource")
		return err
	}
	return nil
}

// createExternalStorageClusterResources creates external cluster resources
func (r *ReconcileStorageCluster) createExternalStorageClusterResources(instance *ocsv1.StorageCluster, reqLogger logr.Logger) error {
	ownerRef := metav1.OwnerReference{
		UID:        instance.UID,
		APIVersion: instance.APIVersion,
		Kind:       instance.Kind,
		Name:       instance.Name,
	}
	scs, err := r.newStorageClasses(instance)
	if err != nil {
		reqLogger.Error(err, "failed to create StorageClasses")
		return err
	}
	// this flag sets the 'ROOK_CSI_ENABLE_CEPHFS' flag
	enableRookCSICephFS := false
	// this stores only the StorageClasses specified in the Secret
	var availableSCs = make([]*storagev1.StorageClass, 3)
	data, err := r.retrieveExternalSecretData(instance, reqLogger)
	if err != nil {
		reqLogger.Error(err, "failed to retrieve external resources")
		return err
	}
	var extCephObjectStores []*cephv1.CephObjectStore
	for _, d := range data {
		objectMeta := metav1.ObjectMeta{
			Name:            d.Name,
			Namespace:       instance.Namespace,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		}
		objectKey := types.NamespacedName{Name: d.Name, Namespace: instance.Namespace}
		switch d.Kind {
		case "CephCluster":
			monitoringIP, ok := d.Data["MonitoringEndpoint"]
			if !ok {
				err := fmt.Errorf(
					"Monitoring Endpoint not present in the external cluster secret %s",
					externalClusterDetailsSecret)
				reqLogger.Error(err, "Failed to get Monitoring IP.")
				return err
			}
			monitoringPort, ok := d.Data["MonitoringPort"]
			if !ok {
				err := fmt.Errorf(
					"Monitoring Port not present in the external cluster secret %s",
					externalClusterDetailsSecret)
				reqLogger.Error(err, "Failed to get Monitoring Port.")
				return err
			}
			err := validateMonitoringEndpoint(monitoringIP, monitoringPort, reqLogger)
			if err != nil {
				reqLogger.Error(err, "Monitoring validation failed")
				return err
			}
			reqLogger.Info("Monitoring Information found. Monitoring will be enabled on the external cluster")
			r.monitoringIP = monitoringIP
		case "ConfigMap":
			cm := &corev1.ConfigMap{
				ObjectMeta: objectMeta,
				Data:       d.Data,
			}
			found := &corev1.ConfigMap{ObjectMeta: objectMeta}
			err := r.createExternalStorageClusterConfigMap(cm, found, reqLogger, objectKey)
			if err != nil {
				reqLogger.Error(err, "could not create ExternalStorageClusterConfigMap")
				return err
			}
		case "Secret":
			sec := &corev1.Secret{
				ObjectMeta: objectMeta,
				Data:       make(map[string][]byte),
			}
			for k, v := range d.Data {
				sec.Data[k] = []byte(v)
			}
			found := &corev1.Secret{ObjectMeta: objectMeta}
			err := r.createExternalStorageClusterSecret(sec, found, reqLogger, objectKey)
			if err != nil {
				reqLogger.Error(err, "could not create ExternalStorageClusterSecret")
				return err
			}
		case "StorageClass":
			index := 0
			var sc *storagev1.StorageClass
			if d.Name == cephFsStorageClassName {
				// 'sc' points to CephFS StorageClass
				index = cephFileSystemIndex
				sc = scs[index]
				enableRookCSICephFS = true
			} else if d.Name == cephRbdStorageClassName {
				// 'sc' points to RBD StorageClass
				index = cephBlockPoolIndex
				sc = scs[index]
			} else if d.Name == cephRgwStorageClassName {
				rgwEndpoint := d.Data[externalCephRgwEndpointKey]
				if err := checkRGWEndpoint(rgwEndpoint, 5*time.Second); err != nil {
					reqLogger.Error(err, fmt.Sprintf("RGW endpoint, %q, is not reachable", rgwEndpoint))
					return err
				}
				extCephObjectStores, err = r.newExternalCephObjectStoreInstances(instance, rgwEndpoint, reqLogger)
				if err != nil {
					return err
				}
				// rgw-endpoint is no longer needed in the 'd.Data' dictionary,
				// and can be deleted
				// created an issue in rook to add `CephObjectStore` type directly in the JSON output
				// https://github.com/rook/rook/issues/6165
				delete(d.Data, externalCephRgwEndpointKey)

				// 'sc' points to OBC StorageClass
				index = cephObjectStoreIndex
				sc = scs[index]
			}
			// now sc is pointing to appropriate StorageClass,
			// whose parameters have to be updated
			for k, v := range d.Data {
				sc.Parameters[k] = v
			}
			availableSCs[index] = sc
		}
	}
	// creating only the available storageClasses
	err = r.createStorageClasses(availableSCs, instance, reqLogger)
	if err != nil {
		reqLogger.Error(err, "failed to create needed StorageClasses")
		return err
	}
	if err = r.setRookCSICephFS(enableRookCSICephFS, instance, reqLogger); err != nil {
		reqLogger.Error(err,
			fmt.Sprintf("failed to set '%s' to %v", rookEnableCephFSCSIKey, enableRookCSICephFS))
		return err
	}
	if extCephObjectStores != nil {
		if err = r.createCephObjectStores(extCephObjectStores, instance, reqLogger); err != nil {
			return err
		}
	}
	return nil
}

// createExternalStorageClusterConfigMap creates configmap for external cluster
func (r *ReconcileStorageCluster) createExternalStorageClusterConfigMap(cm *corev1.ConfigMap, found *corev1.ConfigMap, reqLogger logr.Logger, objectKey types.NamespacedName) error {
	err := r.client.Get(context.TODO(), objectKey, found)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info(fmt.Sprintf("creating configmap: %s", cm.Name))
			err = r.client.Create(context.TODO(), cm)
			if err != nil {
				reqLogger.Error(err, "creation of configmap failed")
				return err
			}
		} else {
			reqLogger.Error(err, "unable the get the configmap")
			return err
		}
	}
	return nil
}

// createExternalStorageClusterSecret creates secret for external cluster
func (r *ReconcileStorageCluster) createExternalStorageClusterSecret(sec *corev1.Secret, found *corev1.Secret, reqLogger logr.Logger, objectKey types.NamespacedName) error {
	err := r.client.Get(context.TODO(), objectKey, found)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info(fmt.Sprintf("creating secret: %s", sec.Name))
			err = r.client.Create(context.TODO(), sec)
			if err != nil {
				reqLogger.Error(err, "creation of secret failed")
				return err
			}
		} else {
			reqLogger.Error(err, "unable the get the secret")
			return err
		}
	}
	return nil
}

// To check if endpoint is a VALID ip and is REACHABLE or not
func validateMonitoringEndpoint(monitoringIP string, monitoringPort string, reqLogger logr.Logger) error {
	_, err := net.LookupIP(monitoringIP)
	if err != nil {
		reqLogger.Error(err, "Monitoring endpoint is not a valid IPv4 IP")
		return err
	}
	endpoint := net.JoinHostPort(monitoringIP, monitoringPort)
	con, err := net.DialTimeout("tcp", endpoint, 5*time.Second)
	if err != nil {
		reqLogger.Error(err, fmt.Sprintf("Monitoring Endpoint (%s) is not reachable", endpoint))
		return err
	}
	defer func() {
		err := con.Close()
		if err != nil {
			log.Error(err, "")
		}
	}()
	return nil
}

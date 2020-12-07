/*
Copyright 2020 Red Hat OpenShift Container Storage.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	nbv1 "github.com/noobaa/noobaa-operator/v2/pkg/apis/noobaa/v1alpha1"
	conditionsv1 "github.com/openshift/custom-resource-status/conditions/v1"
	rook "github.com/rook/rook/pkg/apis/rook.io/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageClusterSpec defines the desired state of StorageCluster
type StorageClusterSpec struct {
	ManageNodes  bool   `json:"manageNodes,omitempty"`
	InstanceType string `json:"instanceType,omitempty"`
	// LabelSelector is used to specify custom labels of nodes to run OCS on
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`
	// External Storage is optional and defaults to false. When set to true, OCS will
	// connect to an external OCS Storage Cluster instead of provisioning one locally.
	ExternalStorage ExternalStorageClusterSpec `json:"externalStorage,omitempty"`
	// HostNetwork defaults to false
	HostNetwork bool `json:"hostNetwork,omitempty"`
	// Placement is optional and used to specify placements of OCS components explicitly
	Placement rook.PlacementSpec `json:"placement,omitempty"`
	// Resources follows the conventions of and is mapped to CephCluster.Spec.Resources
	Resources          map[string]corev1.ResourceRequirements `json:"resources,omitempty"`
	Encryption         EncryptionSpec                         `json:"encryption,omitempty"`
	StorageDeviceSets  []StorageDeviceSet                     `json:"storageDeviceSets,omitempty"`
	MonPVCTemplate     *corev1.PersistentVolumeClaim          `json:"monPVCTemplate,omitempty"`
	MonDataDirHostPath string                                 `json:"monDataDirHostPath,omitempty"`
	MultiCloudGateway  *MultiCloudGatewaySpec                 `json:"multiCloudGateway,omitempty"`
	// Version specifies the version of StorageCluster
	Version string `json:"version,omitempty"`
	// Network represents cluster network settings
	Network *rook.NetworkSpec `json:"network,omitempty"`
	// ManagedResources specifies how to deal with auxiliary resources reconciled
	// with the StorageCluster
	ManagedResources ManagedResourcesSpec `json:"managedResources,omitempty"`
	// If enabled, sets the failureDomain to host, allowing devices to be
	// distributed evenly across all nodes, regardless of distribution in zones
	// or racks.
	FlexibleScaling bool `json:"flexibleScaling,omitempty"`
}

// ManagedResourcesSpec defines how to reconcile auxiliary resources
type ManagedResourcesSpec struct {
	CephBlockPools       ManageCephBlockPools       `json:"cephBlockPools,omitempty"`
	CephFilesystems      ManageCephFilesystems      `json:"cephFilesystems,omitempty"`
	CephObjectStores     ManageCephObjectStores     `json:"cephObjectStores,omitempty"`
	CephObjectStoreUsers ManageCephObjectStoreUsers `json:"cephObjectStoreUsers,omitempty"`
}

// ManageCephBlockPools defines how to reconcilea CephBlockPools
type ManageCephBlockPools struct {
	ReconcileStrategy    string `json:"reconcileStrategy,omitempty"`
	DisableStorageClass  bool   `json:"disableStorageClass,omitempty"`
	DisableSnapshotClass bool   `json:"disableSnapshotClass,omitempty"`
}

// ManageCephFilesystems defines how to reconcile CephFilesystems
type ManageCephFilesystems struct {
	ReconcileStrategy    string `json:"reconcileStrategy,omitempty"`
	DisableStorageClass  bool   `json:"disableStorageClass,omitempty"`
	DisableSnapshotClass bool   `json:"disableSnapshotClass,omitempty"`
}

// ManageCephObjectStores defines how to reconcile CephObjectStores
type ManageCephObjectStores struct {
	ReconcileStrategy   string `json:"reconcileStrategy,omitempty"`
	DisableStorageClass bool   `json:"disableStorageClass,omitempty"`
}

// ManageCephObjectStoreUsers defines how to reconcile CephObjectStoreUsers
type ManageCephObjectStoreUsers struct {
	ReconcileStrategy string `json:"reconcileStrategy,omitempty"`
}

// ExternalStorageClusterSpec defines the spec of the external Storage Cluster
// to be connected to the local cluster
type ExternalStorageClusterSpec struct {
	// +optional
	Enable bool `json:"enable,omitempty"`
}

// StorageDeviceSet defines a set of storage devices.
// It configures the StorageClassDeviceSets field in Rook-Ceph.
type StorageDeviceSet struct {
	// Count is the number of devices in each StorageClassDeviceSet
	// +kubebuilder:validation:Minimum=1
	Count int `json:"count"`

	// Replica is the number of StorageClassDeviceSets for this
	// StorageDeviceSet
	// +kubebuilder:validation:Minimum=1
	// +optional
	Replica int `json:"replica,omitempty"`

	// DeviceType is the value of device type in
	// this StorageDeviceSet. It can have one of the
	// three values (SSD, HDD, NVMe)
	// +kubebuilder:validation:Enum=SSD;ssd;HDD;hdd;NVMe;NVME;nvme
	// +optional
	DeviceType string `json:"deviceType,omitempty"`

	// TopologyKey is the Kubernetes topology label that the
	// StorageClassDeviceSets will be distributed across. Ignored if
	// Placement is set
	// +optional
	TopologyKey string `json:"topologyKey,omitempty"`

	// Portable says whether the OSDs in this device set can move between
	// nodes. This is ignored if Placement is not set
	// +optional
	Portable bool `json:"portable,omitempty"`

	Name                string                        `json:"name"`
	Resources           corev1.ResourceRequirements   `json:"resources,omitempty"`
	PreparePlacement    rook.Placement                `json:"preparePlacement,omitempty"`
	Placement           rook.Placement                `json:"placement,omitempty"`
	Config              StorageDeviceSetConfig        `json:"config,omitempty"`
	DataPVCTemplate     corev1.PersistentVolumeClaim  `json:"dataPVCTemplate"`
	MetadataPVCTemplate *corev1.PersistentVolumeClaim `json:"metadataPVCTemplate,omitempty"`
	WalPVCTemplate      *corev1.PersistentVolumeClaim `json:"walPVCTemplate,omitempty"`
}

// StorageDeviceSetConfig defines Ceph OSD specific config options for the StorageDeviceSet
// TODO: Fill in the members when the actual configurable options are defined in rook-ceph
type StorageDeviceSetConfig struct {
	// TuneSlowDeviceClass tunes the OSD when running on a slow Device Class
	// +optional
	TuneSlowDeviceClass bool `json:"tuneSlowDeviceClass,omitempty"`
}

// MultiCloudGatewaySpec defines specific multi-cloud gateway configuration options
type MultiCloudGatewaySpec struct {
	// ReconcileStrategy specifies whether to reconcile NooBaa CRs. Valid
	// values are "manage", "standalone", "ignore" (same as "standalone"),
	// and "" (same as "manage").
	ReconcileStrategy string `json:"reconcileStrategy,omitempty"`

	// Endpoints (optional) sets configuration info for the noobaa endpoint
	// deployment.
	// +optional
	Endpoints *nbv1.EndpointsSpec `json:"endpoints,omitempty"`
}

// EncryptionSpec defines if encryption should be enabled for the Storage Cluster
// It is optional and defaults to false.
type EncryptionSpec struct {
	// +optional
	Enable bool `json:"enable,omitempty"`
}

// StorageClusterStatus defines the observed state of StorageCluster
type StorageClusterStatus struct {
	// Phase describes the Phase of StorageCluster
	// This is used by OLM UI to provide status information
	// to the user
	Phase string `json:"phase,omitempty"`

	// Conditions describes the state of the StorageCluster resource.
	// +optional
	Conditions []conditionsv1.Condition `json:"conditions,omitempty"`

	// RelatedObjects is a list of objects created and maintained by this
	// operator. Object references will be added to this list after they have
	// been created AND found in the cluster.
	// +optional
	RelatedObjects []corev1.ObjectReference `json:"relatedObjects,omitempty"`

	// NodeTopologies is a list of topology labels on all nodes matching
	// the StorageCluster's placement selector.
	// +optional
	NodeTopologies *NodeTopologyMap `json:"nodeTopologies,omitempty"`

	// FailureDomain is the base CRUSH element Ceph will use to distribute
	// its data replicas for the default CephBlockPool
	// +optional
	FailureDomain string `json:"failureDomain,omitempty"`

	// ExternalSecretHash holds the checksum value of external secret data.
	ExternalSecretHash string `json:"externalSecretHash,omitempty"`
}

// TopologyLabelValues is a list of values for a topology label
type TopologyLabelValues []string

// NodeTopologyMap represents the list of all values of all topology labels
// across all nodes in the StorageCluster
type NodeTopologyMap struct {
	// Labels is a map of topology label keys
	// (e.g. "failure-domain.kubernetes.io") to a set of values for those
	// keys.
	// +optional
	// +nullable
	Labels map[string]TopologyLabelValues `json:"labels,omitempty"`
}

const (
	// ConditionReconcileComplete communicates the status of the StorageCluster resource's
	// reconcile functionality. Basically, is the Reconcile function running to completion.
	ConditionReconcileComplete conditionsv1.ConditionType = "ReconcileComplete"

	// ConditionExternalClusterConnected condition type indicates
	// the successful connection to an external cluster
	ConditionExternalClusterConnected conditionsv1.ConditionType = "ExternalClusterConnected"

	// ConditionExternalClusterConnecting type indicates that rook is still trying for
	// an external connection
	ConditionExternalClusterConnecting conditionsv1.ConditionType = "ExternalClusterConnecting"
)

// List of constants to show different different reconciliation messages and statuses.
const (
	ReconcileFailed                 = "ReconcileFailed"
	ReconcileInit                   = "Init"
	ReconcileCompleted              = "ReconcileCompleted"
	ReconcileCompletedMessage       = "Reconcile completed successfully"
	ExternalClusterConnected        = "ExternalClusterConnected"
	ExternalClusterConnectedMessage = "Connected successfully to an external cluster"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// StorageCluster is the Schema for the storageclusters API
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=.metadata.creationTimestamp
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=.status.phase,description="Current Phase"
// +kubebuilder:printcolumn:name="External",type=boolean,JSONPath=.spec.externalStorage.enable,description="External Storage Cluster"
// +kubebuilder:printcolumn:name="Created At",type=string,JSONPath=.metadata.creationTimestamp
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=.spec.version,description="Storage Cluster Version"
type StorageCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StorageClusterSpec   `json:"spec,omitempty"`
	Status StorageClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// StorageClusterList contains a list of StorageCluster
type StorageClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StorageCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StorageCluster{}, &StorageClusterList{})
}

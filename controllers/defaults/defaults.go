// Package defaults contains the default values for various configurable
// options of a StorageCluster
package defaults

const (
	// NodeAffinityKey is the node label to determine which nodes belong
	// to a storage cluster
	NodeAffinityKey = "cluster.ocs.openshift.io/openshift-storage"
	// NodeTolerationKey is the taint all OCS Pods should tolerate
	NodeTolerationKey = "node.ocs.openshift.io/storage"
	// RackTopologyKey is the node label used to distribute storage nodes
	// when there are not enough AZs presnet across the nodes
	RackTopologyKey = "topology.rook.io/rack"
	// KubeMajorTopologySpreadConstraints is the minimum major kube version to support TSC
	// used along with KubeMinorTSC for version comparison
	KubeMajorTopologySpreadConstraints = "1"
	// KubeMinorTopologySpreadConstraints is the minimum minor kube version to support TSC
	// used along with KubeMajorTSC for version comparison
	KubeMinorTopologySpreadConstraints = "19"
)

var (
	// MonCountMin is the min number of monitors to be configured for the CephCluster
	MonCountMin = 3
	// MonCountMax is the maximum number of monitors to be configured for the CephCluster whenever enough nodes are available
	MonCountMax = 3
	// DeviceSetReplica is the default number of Rook-Ceph
	// StorageClassDeviceSets per StorageCluster StorageDeviceSet
	DeviceSetReplica = 3
)

package collectors

import (
	"context"
	"fmt"

	rgwadmin "github.com/ceph/go-ceph/rgw/admin"
	libbucket "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	bktclient "github.com/kube-object-storage/lib-bucket-provisioner/pkg/client/clientset/versioned"
	"github.com/openshift/ocs-operator/metrics/internal/options"
	"github.com/prometheus/client_golang/prometheus"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
	cephv1listers "github.com/rook/rook/pkg/client/listers/ceph.rook.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

const (
	bucketProvisionerName = "ceph.rook.io-bucket"
	rgwServiceName        = "rook-ceph-rgw"
	svcDNSSuffix          = "svc"
	prometheusUserName    = "prometheus-user"
	endPoint              = "Endpoint"
	accessKey             = "AccessKey"
	secretKey             = "SecretKey"
	cephUser              = "cephUser"
)

var _ prometheus.Collector = &OBCollector{}

// OBCollector is a custom collector for CephObjectStore Custom Resource
type OBCollector struct {
	OBConsumption     *prometheus.Desc
	Informer          cache.SharedIndexInformer
	AllowedNamespaces []string
	bktclient         bktclient.Interface
	rookclient        rookclient.Interface
	k8sclient         kubernetes.Interface
}

// NewOBCollector constructs a collector
func NewOBCollector(opts *options.Options) *OBCollector {

	sharedIndexInformer := CephObjectStoreInformer(opts)

	return &OBCollector{
		OBConsumption: prometheus.NewDesc(
			"obc_metrics",
			`Metrics for OBC. no of objects, total size consumed`,
			[]string{"name", "rgw_endpoint"},
			nil,
		),
		Informer:          sharedIndexInformer,
		AllowedNamespaces: opts.AllowedNamespaces,
		bktclient:         bktclient.NewForConfigOrDie(opts.Kubeconfig),
		rookclient:        rookclient.NewForConfigOrDie(opts.Kubeconfig),
		k8sclient:         kubernetes.NewForConfigOrDie(opts.Kubeconfig),
	}
}

// Run starts CephObjectStore informer
func (c *OBCollector) Run(stopCh <-chan struct{}) {
	go c.Informer.Run(stopCh)
}

// Describe implements prometheus.Collector interface
func (c *OBCollector) Describe(ch chan<- *prometheus.Desc) {
	ds := []*prometheus.Desc{
		c.OBConsumption,
	}

	for _, d := range ds {
		ch <- d
	}
}

// Collect implements prometheus.Collector interface
func (c *OBCollector) Collect(ch chan<- prometheus.Metric) {
	cephObjectStoreLister := cephv1listers.NewCephObjectStoreLister(c.Informer.GetIndexer())
	cephObjectStores := getAllObjectStores(cephObjectStoreLister, c.AllowedNamespaces)
	if len(cephObjectStores) > 0 {
		c.collectObjectBucketMetricsSize(cephObjectStores, ch)
	}
}

func (c *OBCollector) getAllObjectBuckets(name, namespace string) (objectBuckets *libbucket.ObjectBucketList) {
	selector := fmt.Sprintf("bucket-provisioner=%s.%s", namespace, bucketProvisionerName)
	fieldSelector := fmt.Sprintf("spec.endpoint.bucketHost=%s.%s.%s.%s", rgwServiceName, name, namespace, svcDNSSuffix)
	listOpts := metav1.ListOptions{
		LabelSelector: selector,
		FieldSelector: fieldSelector,
	}

	objectBuckets, err := c.bktclient.ObjectbucketV1alpha1().ObjectBuckets().List(context.TODO(), listOpts)
	if err != nil {
		klog.Errorf("couldn't list ObjectBuckets. %v", err)
	}
	return
}

func (c *OBCollector) collectObjectBucketMetricsSize(cephObjectStores []*cephv1.CephObjectStore, ch chan<- prometheus.Metric) {
	ctx := context.TODO()
	for _, cephObjectStore := range cephObjectStores {
		objectBuckets := c.getAllObjectBuckets(cephObjectStore.Name, cephObjectStore.Namespace)
		if len(objectBuckets.Items) > 0 {
			prometheusSecretName := fmt.Sprintf("rook-ceph-object-user-%s-%s", prometheusUserName, cephObjectStore.Name)

			secret, _ := c.k8sclient.CoreV1().Secrets(cephObjectStore.Namespace).Get(ctx, prometheusSecretName, metav1.GetOptions{})
			//TODO: SSL endpoint
			if secret != nil {
				adminAPI := rgwadmin.New(secret.Data[endpoint], secret.Data[accessKey], secret.Data[secretKey], nil)
				for _, ob := range objectBuckets.Items {
					quotainfo := adminAPI.GetUserQuota(ctx, rgwadmin.QuotaSpec{UID: ob.Spec.AdditionalState[cephUser]})

					ch <- prometheus.MustNewConstMetric(c.OBConsumption, prometheus.CounterValue, float64(quotainfo.MaxSizeKb), ob.Name, secret.Data[endpoint])
				}
			} else {
				klog.Error("CephObjectStoreUser for collecting promethues metrics not found")
			}
			/* if secret is not found, do we need to create user from here?
				    objectUser := rookclient.CephObjectStoreUser{
			        ObjectMeta: metav1.ObjectMeta{
			            Name:      prometheusUserName,
			            Namespace: cephObjectStore.Namespace,
			        },
			        Spec: cephv1.ObjectStoreUserSpec{
			            Store: cephObjectStore.Name,
			        },
			        TypeMeta: metav1.TypeMeta{
			            Kind: "CephObjectStoreUser",
			        },
			    }

					prometheusUser, err := c.rookclient.CephV1().CephObjectStoreUsers(cephObjectStore.Namespace).Create(ctx, objectUser, metav1.CreateOptions{})
			*/
		} else {
			klog.Infof("Zero OB present for object store %s", cephObjectStore.Name)
		}
	}
}

// TODO : Get of no of objects as well

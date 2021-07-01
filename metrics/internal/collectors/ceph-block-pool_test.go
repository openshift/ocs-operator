package collectors

import (
	"testing"

	"github.com/openshift/ocs-operator/metrics/internal/options"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephv1listers "github.com/rook/rook/pkg/client/listers/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	mockOptsCBP = &options.Options{
		Apiserver:         "https://localhost:8443",
		KubeconfigPath:    "",
		Host:              "0.0.0.0",
		Port:              8080,
		ExporterHost:      "0.0.0.0",
		ExporterPort:      8081,
		AllowedNamespaces: []string{"openshift-storage"},
		Help:              false,
	}
	mockCephBlockPool1 = cephv1.CephBlockPool{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "ceph.rook.io/v1",
			Kind:       "CephBlockPool",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mockCephBlockPool-1",
			Namespace: "openshift-storage",
		},
		Spec:   cephv1.PoolSpec{},
		Status: &cephv1.CephBlockPoolStatus{},
	}
	mockCephBlockPool2 = cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mockCephBlockPool-2",
			Namespace: "openshift-storage",
		},
		Spec:   cephv1.PoolSpec{},
		Status: &cephv1.CephBlockPoolStatus{},
	}
)

func setKubeConfigCBP(t *testing.T) {
	kubeconfig, err := clientcmd.BuildConfigFromFlags(mockOptsCBP.Apiserver, mockOpts.KubeconfigPath)
	assert.Nil(t, err, "error: %v", err)

	mockOptsCBP.Kubeconfig = kubeconfig
}

func getMockCephBlockPoolCollector(t *testing.T, mockOptsCBP *options.Options) (mockCephBlockPoolCollector *CephBlockPoolCollector) {
	setKubeConfigCBP(t)
	mockCephBlockPoolCollector = NewCephBlockPoolCollector(mockOptsCBP)
	assert.NotNil(t, mockCephBlockPoolCollector)
	return
}

func setInformerStoreCBP(t *testing.T, objs []*cephv1.CephBlockPool, mockCephBlockPoolCollector *CephBlockPoolCollector) {
	for _, obj := range objs {
		err := mockCephBlockPoolCollector.Informer.GetStore().Add(obj)
		assert.Nil(t, err)
	}
}

func resetInformerStoreCBP(t *testing.T, objs []*cephv1.CephBlockPool, mockCephBlockPoolCollector *CephBlockPoolCollector) {
	for _, obj := range objs {
		err := mockCephBlockPoolCollector.Informer.GetStore().Delete(obj)
		assert.Nil(t, err)
	}
}

func TestNewCephBlockPoolCollector(t *testing.T) {
	type args struct {
		opts *options.Options
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "Test CephBlockPoolCollector",
			args: args{
				opts: mockOptsCBP,
			},
		},
	}
	for _, tt := range tests {
		got := getMockCephBlockPoolCollector(t, tt.args.opts)
		assert.NotNil(t, got.AllowedNamespaces)
		assert.NotNil(t, got.Informer)
	}
}

func TestGetAllBlockPools(t *testing.T) {
	mockOptsCBP.StopCh = make(chan struct{})
	defer close(mockOptsCBP.StopCh)

	cephBlockPoolCollector := getMockCephBlockPoolCollector(t, mockOptsCBP)

	type args struct {
		lister     cephv1listers.CephBlockPoolLister
		namespaces []string
	}
	tests := []struct {
		name                string
		args                args
		inputCephBlockPools []*cephv1.CephBlockPool
		wantCephBlockPools  []*cephv1.CephBlockPool
	}{
		{
			name: "CephBlockPools doesn't exist",
			args: args{
				lister:     cephv1listers.NewCephBlockPoolLister(cephBlockPoolCollector.Informer.GetIndexer()),
				namespaces: cephBlockPoolCollector.AllowedNamespaces,
			},
			inputCephBlockPools: []*cephv1.CephBlockPool{},
			// []*cephv1.CephBlockPool(nil) is not DeepEqual to []*cephv1.CephBlockPool{}
			// getAllBlockPools returns []*cephv1.CephBlockPool(nil) if no CephBlockPool is present
			wantCephBlockPools: []*cephv1.CephBlockPool(nil),
		},
		{
			name: "One CephBlockPools exists",
			args: args{
				lister:     cephv1listers.NewCephBlockPoolLister(cephBlockPoolCollector.Informer.GetIndexer()),
				namespaces: cephBlockPoolCollector.AllowedNamespaces,
			},
			inputCephBlockPools: []*cephv1.CephBlockPool{
				&mockCephBlockPool1,
			},
			wantCephBlockPools: []*cephv1.CephBlockPool{
				&mockCephBlockPool1,
			},
		},
		{
			name: "Two CephBlockPools exists",
			args: args{
				lister:     cephv1listers.NewCephBlockPoolLister(cephBlockPoolCollector.Informer.GetIndexer()),
				namespaces: cephBlockPoolCollector.AllowedNamespaces,
			},
			inputCephBlockPools: []*cephv1.CephBlockPool{
				&mockCephBlockPool1,
				&mockCephBlockPool2,
			},
			wantCephBlockPools: []*cephv1.CephBlockPool{
				&mockCephBlockPool1,
				&mockCephBlockPool2,
			},
		},
	}
	for _, tt := range tests {
		setInformerStoreCBP(t, tt.inputCephBlockPools, cephBlockPoolCollector)
		gotCephBlockPools := getAllBlockPools(tt.args.lister, tt.args.namespaces)
		assert.Len(t, gotCephBlockPools, len(tt.wantCephBlockPools))
		for _, obj := range gotCephBlockPools {
			assert.Contains(t, tt.wantCephBlockPools, obj)
		}
		resetInformerStoreCBP(t, tt.inputCephBlockPools, cephBlockPoolCollector)
	}
}

func TestCollectPoolMirroringImageHealth(t *testing.T) {
	mockOptsCBP.StopCh = make(chan struct{})
	defer close(mockOptsCBP.StopCh)

	cephBlockPoolCollector := getMockCephBlockPoolCollector(t, mockOptsCBP)

	objOk := mockCephBlockPool1.DeepCopy()
	objOk.Name = objOk.Name + "ok"
	objOk.Status = &cephv1.CephBlockPoolStatus{
		MirroringStatus: &cephv1.MirroringStatusSpec{PoolMirroringStatus: cephv1.PoolMirroringStatus{Summary: &cephv1.PoolMirroringStatusSummarySpec{ImageHealth: "OK"}}},
	}

	objWarning := mockCephBlockPool1.DeepCopy()
	objWarning.Name = objWarning.Name + "warning"
	objWarning.Status = &cephv1.CephBlockPoolStatus{
		MirroringStatus: &cephv1.MirroringStatusSpec{PoolMirroringStatus: cephv1.PoolMirroringStatus{Summary: &cephv1.PoolMirroringStatusSummarySpec{ImageHealth: "WARNING"}}},
	}

	objError := mockCephBlockPool1.DeepCopy()
	objError.Name = objError.Name + "error"
	objError.Status = &cephv1.CephBlockPoolStatus{
		MirroringStatus: &cephv1.MirroringStatusSpec{PoolMirroringStatus: cephv1.PoolMirroringStatus{Summary: &cephv1.PoolMirroringStatusSummarySpec{ImageHealth: "ERROR"}}},
	}

	objUnknown := mockCephBlockPool1.DeepCopy()
	objUnknown.Name = objError.Name + "unknown"
	objUnknown.Status = &cephv1.CephBlockPoolStatus{
		MirroringStatus: &cephv1.MirroringStatusSpec{PoolMirroringStatus: cephv1.PoolMirroringStatus{Summary: &cephv1.PoolMirroringStatusSummarySpec{ImageHealth: "UNKNOWN"}}},
	}

	type args struct {
		cephBlockPools []*cephv1.CephBlockPool
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "Collect CephBlockPool mirroring image health metrics",
			args: args{
				cephBlockPools: []*cephv1.CephBlockPool{
					objOk,
					objWarning,
					objError,
					objUnknown,
				},
			},
		},
		{
			name: "Empty CephBlockPool",
			args: args{
				cephBlockPools: []*cephv1.CephBlockPool{},
			},
		},
	}
	// Image health
	for _, tt := range tests {
		ch := make(chan prometheus.Metric)
		metric := dto.Metric{}
		go func() {
			cephBlockPoolCollector.collectMirroringImageHealth(tt.args.cephBlockPools, ch)
			close(ch)
		}()

		for m := range ch {
			assert.Contains(t, m.Desc().String(), "image_health")
			metric.Reset()
			err := m.Write(&metric)
			assert.Nil(t, err)
			labels := metric.GetLabel()
			for _, label := range labels {
				if *label.Name == "name" {
					if *label.Value == objOk.Name {
						assert.Equal(t, *metric.Gauge.Value, float64(0))
					} else if *label.Value == objWarning.Name {
						assert.Equal(t, *metric.Gauge.Value, float64(1))
					} else if *label.Value == objError.Name {
						assert.Equal(t, *metric.Gauge.Value, float64(2))
					}
				} else if *label.Name == "namespace" {
					assert.Contains(t, cephBlockPoolCollector.AllowedNamespaces, *label.Value)
				}
			}
		}
	}

}

func TestCollectPoolMirroringStatus(t *testing.T) {
	mockOptsCBP.StopCh = make(chan struct{})
	defer close(mockOptsCBP.StopCh)

	cephBlockPoolCollector := getMockCephBlockPoolCollector(t, mockOptsCBP)

	objEnabled := mockCephBlockPool1.DeepCopy()
	objEnabled.Name = objEnabled.Name + "enabled"
	objEnabled.Spec = cephv1.PoolSpec{
		Mirroring: cephv1.MirroringSpec{Enabled: true},
	}

	objDisabled := mockCephBlockPool1.DeepCopy()
	objDisabled.Name = objDisabled.Name + "disabled"
	objDisabled.Spec = cephv1.PoolSpec{
		Mirroring: cephv1.MirroringSpec{Enabled: false},
	}

	type args struct {
		cephBlockPools []*cephv1.CephBlockPool
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "Collect CephBlockPool mirroring status",
			args: args{
				cephBlockPools: []*cephv1.CephBlockPool{
					objEnabled,
					objDisabled,
				},
			},
		},
		{
			name: "Empty CephBlockPool",
			args: args{
				cephBlockPools: []*cephv1.CephBlockPool{},
			},
		},
	}
	// Image health
	for _, tt := range tests {
		ch := make(chan prometheus.Metric)
		metric := dto.Metric{}
		go func() {
			cephBlockPoolCollector.collectMirroringStatus(tt.args.cephBlockPools, ch)
			close(ch)
		}()

		for m := range ch {
			assert.Contains(t, m.Desc().String(), "status")
			metric.Reset()
			err := m.Write(&metric)
			assert.Nil(t, err)
			labels := metric.GetLabel()
			for _, label := range labels {
				if *label.Name == "name" {
					if *label.Value == objEnabled.Name {
						assert.Equal(t, *metric.Gauge.Value, float64(0))
					} else if *label.Value == objDisabled.Name {
						assert.Equal(t, *metric.Gauge.Value, float64(1))
					}
				} else if *label.Name == "namespace" {
					assert.Contains(t, cephBlockPoolCollector.AllowedNamespaces, *label.Value)
				}
			}
		}
	}

}

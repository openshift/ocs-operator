package ocs_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	ocsv1 "github.com/openshift/ocs-operator/api/v1"
	tests "github.com/openshift/ocs-operator/functests"
	deploymanager "github.com/openshift/ocs-operator/internal/deploy-manager"

	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	disableToolsPatch = `[{ "op": "replace", "path": "/spec/enableCephTools", "value": false }]`
	enableToolsPatch  = `[{ "op": "replace", "path": "/spec/enableCephTools", "value": true }]`
)

type RookCephTools struct {
	k8sClient      *kubernetes.Clientset
	ocsClient      *rest.RESTClient
	parameterCodec runtime.ParameterCodec
	namespace      string
}

func newRookCephTools() (*RookCephTools, error) {
	retOCSObj := &RookCephTools{
		k8sClient:      tests.DeployManager.GetK8sClient(),
		ocsClient:      tests.DeployManager.GetOcsClient(),
		parameterCodec: tests.DeployManager.GetParameterCodec(),
		namespace:      tests.DeployManager.GetNamespace(),
	}
	return retOCSObj, nil
}

func (rctObj *RookCephTools) patchOCSInit(patch string) error {
	init := &ocsv1.OCSInitialization{}
	return rctObj.ocsClient.Patch(types.JSONPatchType).
		Resource("ocsinitializations").
		Namespace(rctObj.namespace).
		Name("ocsinit").
		Body([]byte(patch)).
		VersionedParams(&metav1.GetOptions{}, rctObj.parameterCodec).
		Do(context.TODO()).
		Into(init)
}

func (rctObj *RookCephTools) toolsPodOnlineCheck() error {
	pods, err := rctObj.k8sClient.CoreV1().Pods(deploymanager.InstallNamespace).List(context.TODO(), metav1.ListOptions{LabelSelector: "app=rook-ceph-tools"})
	if err != nil {
		return err
	}
	if len(pods.Items) == 0 {
		return fmt.Errorf("waiting on a rook-tools-pod to come online")
	}
	if pods.Items[0].Status.Phase != k8sv1.PodRunning {
		return fmt.Errorf("Waiting on rook-tools-pod with phase %s to be %s",
			pods.Items[0].Status.Phase, k8sv1.PodRunning)
	}
	// pod is online and running
	return nil
}

func (rctObj *RookCephTools) toolsRemove() error {
	pods, err := rctObj.k8sClient.CoreV1().Pods(deploymanager.InstallNamespace).List(context.TODO(), metav1.ListOptions{LabelSelector: "app=rook-ceph-tools"})
	if err != nil {
		return err
	}
	if len(pods.Items) != 0 {
		return fmt.Errorf("waiting for rook-tools-pod to be deleted")
	}
	// pod is removed
	return nil
}

var _ = Describe("Rook Ceph Tools", rookCephToolsTest)

func rookCephToolsTest() {
	var rctObj *RookCephTools
	var err error

	BeforeEach(func() {
		rctObj, err = newRookCephTools()
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			tests.SuiteFailed = tests.SuiteFailed || true
		}
	})

	Describe("Deployment", func() {
		AfterEach(func() {
			err = rctObj.patchOCSInit(disableToolsPatch)
			Expect(err).To(BeNil())
		})

		It("Ensure enable tools works", func() {
			By("Setting enableCephTools=true")
			err = rctObj.patchOCSInit(enableToolsPatch)
			Expect(err).To(BeNil())

			By("Ensuring tools are created")
			Eventually(rctObj.toolsPodOnlineCheck, 200*time.Second, 1*time.Second).ShouldNot(HaveOccurred())

			By("Setting enableCephTools=false")
			err = rctObj.patchOCSInit(disableToolsPatch)
			Expect(err).To(BeNil())

			By("Ensuring tools are removed")
			Eventually(rctObj.toolsRemove, 200*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
		})
	})
}

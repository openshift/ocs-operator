package deploymanager

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	yaml "github.com/ghodss/yaml"
	v1 "github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1"
	v1alpha1 "github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilwait "k8s.io/apimachinery/pkg/util/wait"
)

const localStorageNamespace = "local-storage"
const marketplaceNamespace = "openshift-marketplace"
const defaultLocalStorageRegistryImage = "quay.io/gnufied/local-registry:v4.2.0"
const defaultOcsRegistryImage = "quay.io/ocs-dev/ocs-registry:latest"

type clusterObjects struct {
	namespaces     []k8sv1.Namespace
	operatorGroups []v1.OperatorGroup
	catalogSources []v1alpha1.CatalogSource
	subscriptions  []v1alpha1.Subscription
}

func (t *DeployManager) deployClusterObjects(co *clusterObjects) error {

	for _, namespace := range co.namespaces {
		err := t.CreateNamespace(namespace.Name)
		if err != nil {
			return err
		}
	}

	for _, operatorGroup := range co.operatorGroups {
		_, err := t.olmClient.OperatorsV1().OperatorGroups(operatorGroup.Namespace).Create(&operatorGroup)
		if err != nil && !errors.IsAlreadyExists(err) {
			return err
		}

	}

	for _, catalogSource := range co.catalogSources {
		_, err := t.olmClient.OperatorsV1alpha1().CatalogSources(catalogSource.Namespace).Create(&catalogSource)
		if err != nil && !errors.IsAlreadyExists(err) {
			return err
		}
	}

	// Wait for catalog source before posting subscription
	err := t.waitForOCSCatalogSource()
	if err != nil {
		return err
	}

	for _, subscription := range co.subscriptions {
		_, err := t.olmClient.OperatorsV1alpha1().Subscriptions(subscription.Namespace).Create(&subscription)
		if err != nil && !errors.IsAlreadyExists(err) {
			return err
		}
	}

	// Wait on ocs-operator, rook-ceph-operator and noobaa-operator to come online.
	err = t.waitForOCSOperator()
	if err != nil {
		return err
	}

	return nil
}

func (t *DeployManager) generateClusterObjects(ocsRegistryImage string, localStorageRegistryImage string) *clusterObjects {

	co := &clusterObjects{}
	label := make(map[string]string)
	// Label required for monitoring this namespace
	label["openshift.io/cluster-monitoring"] = "true"

	// Namespaces
	co.namespaces = append(co.namespaces, k8sv1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   InstallNamespace,
			Labels: label,
		},
	})
	co.namespaces = append(co.namespaces, k8sv1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: localStorageNamespace,
		},
	})

	// Operator Groups
	ocsOG := v1.OperatorGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openshift-storage-operatorgroup",
			Namespace: InstallNamespace,
		},
		Spec: v1.OperatorGroupSpec{
			TargetNamespaces: []string{InstallNamespace},
		},
	}
	ocsOG.SetGroupVersionKind(schema.GroupVersionKind{Group: v1.SchemeGroupVersion.Group, Kind: "OperatorGroup", Version: v1.SchemeGroupVersion.Version})

	localStorageOG := v1.OperatorGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "local-operator-group",
			Namespace: localStorageNamespace,
		},
		Spec: v1.OperatorGroupSpec{
			TargetNamespaces: []string{InstallNamespace},
		},
	}
	localStorageOG.SetGroupVersionKind(schema.GroupVersionKind{Group: v1.SchemeGroupVersion.Group, Kind: "OperatorGroup", Version: v1.SchemeGroupVersion.Version})

	co.operatorGroups = append(co.operatorGroups, ocsOG)
	co.operatorGroups = append(co.operatorGroups, localStorageOG)

	// CatalogSources
	localStorageCatalog := v1alpha1.CatalogSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "local-storage-manifests",
			Namespace: marketplaceNamespace,
		},
		Spec: v1alpha1.CatalogSourceSpec{
			SourceType:  v1alpha1.SourceTypeGrpc,
			Image:       localStorageRegistryImage,
			DisplayName: "Local Storage Operator",
			Publisher:   "Red Hat",
			Description: "An operator to manage local volumes",
		},
	}
	localStorageCatalog.SetGroupVersionKind(schema.GroupVersionKind{Group: v1alpha1.SchemeGroupVersion.Group, Kind: "CatalogSource", Version: v1alpha1.SchemeGroupVersion.Version})

	ocsCatalog := v1alpha1.CatalogSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ocs-catalogsource",
			Namespace: marketplaceNamespace,
		},
		Spec: v1alpha1.CatalogSourceSpec{
			SourceType:  v1alpha1.SourceTypeGrpc,
			Image:       ocsRegistryImage,
			DisplayName: "OpenShift Container Storage",
			Publisher:   "Red Hat",
			Icon: v1alpha1.Icon{
				Data:      "PHN2ZyBpZD0iTGF5ZXJfMSIgZGF0YS1uYW1lPSJMYXllciAxIiB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAxOTIgMTQ1Ij48ZGVmcz48c3R5bGU+LmNscy0xe2ZpbGw6I2UwMDt9PC9zdHlsZT48L2RlZnM+PHRpdGxlPlJlZEhhdC1Mb2dvLUhhdC1Db2xvcjwvdGl0bGU+PHBhdGggZD0iTTE1Ny43Nyw2Mi42MWExNCwxNCwwLDAsMSwuMzEsMy40MmMwLDE0Ljg4LTE4LjEsMTcuNDYtMzAuNjEsMTcuNDZDNzguODMsODMuNDksNDIuNTMsNTMuMjYsNDIuNTMsNDRhNi40Myw2LjQzLDAsMCwxLC4yMi0xLjk0bC0zLjY2LDkuMDZhMTguNDUsMTguNDUsMCwwLDAtMS41MSw3LjMzYzAsMTguMTEsNDEsNDUuNDgsODcuNzQsNDUuNDgsMjAuNjksMCwzNi40My03Ljc2LDM2LjQzLTIxLjc3LDAtMS4wOCwwLTEuOTQtMS43My0xMC4xM1oiLz48cGF0aCBjbGFzcz0iY2xzLTEiIGQ9Ik0xMjcuNDcsODMuNDljMTIuNTEsMCwzMC42MS0yLjU4LDMwLjYxLTE3LjQ2YTE0LDE0LDAsMCwwLS4zMS0zLjQybC03LjQ1LTMyLjM2Yy0xLjcyLTcuMTItMy4yMy0xMC4zNS0xNS43My0xNi42QzEyNC44OSw4LjY5LDEwMy43Ni41LDk3LjUxLjUsOTEuNjkuNSw5MCw4LDgzLjA2LDhjLTYuNjgsMC0xMS42NC01LjYtMTcuODktNS42LTYsMC05LjkxLDQuMDktMTIuOTMsMTIuNSwwLDAtOC40MSwyMy43Mi05LjQ5LDI3LjE2QTYuNDMsNi40MywwLDAsMCw0Mi41Myw0NGMwLDkuMjIsMzYuMywzOS40NSw4NC45NCwzOS40NU0xNjAsNzIuMDdjMS43Myw4LjE5LDEuNzMsOS4wNSwxLjczLDEwLjEzLDAsMTQtMTUuNzQsMjEuNzctMzYuNDMsMjEuNzdDNzguNTQsMTA0LDM3LjU4LDc2LjYsMzcuNTgsNTguNDlhMTguNDUsMTguNDUsMCwwLDEsMS41MS03LjMzQzIyLjI3LDUyLC41LDU1LC41LDc0LjIyYzAsMzEuNDgsNzQuNTksNzAuMjgsMTMzLjY1LDcwLjI4LDQ1LjI4LDAsNTYuNy0yMC40OCw1Ni43LTM2LjY1LDAtMTIuNzItMTEtMjcuMTYtMzAuODMtMzUuNzgiLz48L3N2Zz4=",
				MediaType: "image/svg+xml",
			},
		},
	}
	ocsCatalog.SetGroupVersionKind(schema.GroupVersionKind{Group: v1alpha1.SchemeGroupVersion.Group, Kind: "CatalogSource", Version: v1alpha1.SchemeGroupVersion.Version})

	co.catalogSources = append(co.catalogSources, localStorageCatalog)
	co.catalogSources = append(co.catalogSources, ocsCatalog)

	// Subscriptions
	ocsSubscription := v1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ocs-subscription",
			Namespace: InstallNamespace,
		},
		Spec: &v1alpha1.SubscriptionSpec{
			Channel:                "alpha",
			Package:                "ocs-operator",
			CatalogSource:          "ocs-catalogsource",
			CatalogSourceNamespace: marketplaceNamespace,
		},
	}
	ocsSubscription.SetGroupVersionKind(schema.GroupVersionKind{Group: v1alpha1.SchemeGroupVersion.Group, Kind: "Subscription", Version: v1alpha1.SchemeGroupVersion.Version})

	co.subscriptions = append(co.subscriptions, ocsSubscription)

	return co
}

func marshallObject(obj interface{}, writer io.Writer) error {
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	var r unstructured.Unstructured
	if err := json.Unmarshal(jsonBytes, &r.Object); err != nil {
		return err
	}

	unstructured.RemoveNestedField(r.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(r.Object, "status")

	jsonBytes, err = json.Marshal(r.Object)
	if err != nil {
		return err
	}

	yamlBytes, err := yaml.JSONToYAML(jsonBytes)
	if err != nil {
		return err
	}

	// fix double quoted strings by removing unneeded single quotes...
	s := string(yamlBytes)
	s = strings.Replace(s, " '\"", " \"", -1)
	s = strings.Replace(s, "\"'\n", "\"\n", -1)

	yamlBytes = []byte(s)

	_, err = writer.Write([]byte("---\n"))
	if err != nil {
		return err
	}

	_, err = writer.Write(yamlBytes)
	if err != nil {
		return err
	}

	return nil
}

// DumpYAML dumps ocs deployment yaml
func (t *DeployManager) DumpYAML(ocsRegistryImage string, localStorageRegistryImage string) string {
	co := t.generateClusterObjects(ocsRegistryImage, localStorageRegistryImage)

	writer := strings.Builder{}

	for _, namespace := range co.namespaces {
		marshallObject(namespace, &writer)
	}

	for _, operatorGroup := range co.operatorGroups {
		marshallObject(operatorGroup, &writer)
	}

	for _, catalogSource := range co.catalogSources {
		marshallObject(catalogSource, &writer)
	}

	for _, subscription := range co.subscriptions {
		marshallObject(subscription, &writer)
	}

	return writer.String()
}

func (t *DeployManager) waitForOCSCatalogSource() error {
	timeout := 300 * time.Second
	interval := 10 * time.Second

	lastReason := ""

	labelSelector, err := labels.Parse("olm.catalogSource in (ocs-catalogsource)")
	if err != nil {
		return err
	}

	err = utilwait.PollImmediate(interval, timeout, func() (done bool, err error) {
		pods, err := t.k8sClient.CoreV1().Pods(marketplaceNamespace).List(metav1.ListOptions{
			LabelSelector: labelSelector.String(),
		})
		if err != nil {
			lastReason = fmt.Sprintf("error talking to k8s apiserver: %v", err)
			return false, nil
		}

		if len(pods.Items) == 0 {
			lastReason = "waiting on ocs catalog source pod to be created"
			return false, nil
		}
		isReady := false
		for _, pod := range pods.Items {
			for _, condition := range pod.Status.Conditions {
				if condition.Type == k8sv1.PodReady && condition.Status == k8sv1.ConditionTrue {
					isReady = true
					break
				}
			}
		}

		if !isReady {
			lastReason = "waiting on ocs catalog source pod to reach ready state"
			return false, nil
		}

		// if we get here, then all deployments are created and available
		return true, nil
	})

	if err != nil {
		return fmt.Errorf("%v: %s", err, lastReason)
	}

	return nil
}

// DeployOCSWithOLM deploys ocs operator via an olm subscription
func (t *DeployManager) DeployOCSWithOLM(ocsRegistryImage string, localStorageRegistryImage string) error {

	if ocsRegistryImage == "" || localStorageRegistryImage == "" {
		return fmt.Errorf("catalog registry images not supplied")
	}

	co := t.generateClusterObjects(ocsRegistryImage, localStorageRegistryImage)
	err := t.deployClusterObjects(co)
	if err != nil {
		return err
	}

	return nil
}

func (t *DeployManager) waitForOCSOperator() error {
	deployments := []string{"ocs-operator", "rook-ceph-operator", "noobaa-operator"}

	timeout := 1000 * time.Second
	interval := 10 * time.Second

	lastReason := ""

	err := utilwait.PollImmediate(interval, timeout, func() (done bool, err error) {
		for _, name := range deployments {
			deployment, err := t.k8sClient.AppsV1().Deployments(InstallNamespace).Get(name, metav1.GetOptions{})
			if err != nil {
				lastReason = fmt.Sprintf("waiting on deployment %s to be created", name)
				return false, nil
			}

			isAvailable := false
			for _, condition := range deployment.Status.Conditions {
				if condition.Type == appsv1.DeploymentAvailable && condition.Status == k8sv1.ConditionTrue {
					isAvailable = true
					break
				}
			}

			if !isAvailable {
				lastReason = fmt.Sprintf("waiting on deployment %s to become available", name)
				return false, nil
			}
		}

		// if we get here, then all deployments are created and available
		return true, nil
	})

	if err != nil {
		return fmt.Errorf("%v: %s", err, lastReason)
	}

	return nil
}

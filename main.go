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

package main

import (
	"context"
	"flag"
	"os"

	monitoringv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	snapapi "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	nbapis "github.com/noobaa/noobaa-operator/v2/pkg/apis"
	openshiftConfigv1 "github.com/openshift/api/config/v1"
	openshiftv1 "github.com/openshift/api/template/v1"
	ocsv1 "github.com/openshift/ocs-operator/api/v1"
	"github.com/openshift/ocs-operator/controllers/ocsinitialization"
	"github.com/openshift/ocs-operator/controllers/storagecluster"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("main")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(ocsv1.AddToScheme(scheme))
	utilruntime.Must(cephv1.AddToScheme(scheme))
	utilruntime.Must(monitoringv1.AddToScheme(scheme))
	utilruntime.Must(snapapi.AddToScheme(scheme))
	utilruntime.Must(nbapis.AddToScheme(scheme))
	utilruntime.Must(openshiftConfigv1.AddToScheme(scheme))
	utilruntime.Must(openshiftv1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(storagev1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	// isDevelopmentEnv is a command line option that takes boolean value.
	// It defaults to 'false' and indicates if the cluster is running in Production
	// or not. This helps us configure logger accordingly.
	var isDevelopmentEnv bool

	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&isDevelopmentEnv, "development", false, "Enable/Disable running operator in development environment")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(isDevelopmentEnv)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "ab76f4c9.openshift.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&ocsinitialization.OCSInitializationReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("OCSInitialization"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OCSInitialization")
		os.Exit(1)
	}
	if err = (&storagecluster.StorageClusterReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("StorageCluster"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "StorageCluster")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	// Create CR if it's not there
	ocsNamespacedName := ocsinitialization.InitNamespacedName()
	client := mgr.GetClient()
	err = client.Create(context.TODO(), &ocsv1.OCSInitialization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ocsNamespacedName.Name,
			Namespace: ocsNamespacedName.Namespace,
		},
	})

	switch {
	case err == nil:
		setupLog.Info("Created OCSInitialization resource")
	case errors.IsAlreadyExists(err):
		setupLog.Info("OCSInitialization resource already exists")
	default:
		setupLog.Error(err, "Failed to create OCSInitialization custom resource")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

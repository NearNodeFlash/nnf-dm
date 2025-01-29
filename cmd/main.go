/*
 * Copyright 2021-2025 Hewlett Packard Enterprise Development LP
 * Other additional copyright holders may be indicated within.
 *
 * The entirety of this work is licensed under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 *
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"flag"
	"os"
	"strconv"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	zapcr "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	lusv1beta1 "github.com/NearNodeFlash/lustre-fs-operator/api/v1beta1"
	nnfv1alpha5 "github.com/NearNodeFlash/nnf-sos/api/v1alpha5"

	controllers "github.com/NearNodeFlash/nnf-dm/internal/controller"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(lusv1beta1.AddToScheme(scheme))
	utilruntime.Must(nnfv1alpha5.AddToScheme(scheme))

	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var controller string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&controller, "controller", "default", "The controller type to run {manager, node, default}.")

	flag.Parse()

	encoder := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	zaplogger := zapcr.New(zapcr.Encoder(encoder), zapcr.UseDevMode(true))

	ctrl.SetLogger(zaplogger)

	dmCtrl := newDataMovementController(controller)

	options := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "a60dd315.cray.hpe.com",
	}

	dmCtrl.SetOptions(&options)

	config := ctrl.GetConfigOrDie()
	qpsString, found := os.LookupEnv("NNF_REST_CONFIG_QPS")
	if found {
		qps, err := strconv.ParseFloat(qpsString, 32)
		if err != nil {
			setupLog.Error(err, "invalid value for NNF_REST_CONFIG_QPS")
			os.Exit(1)
		}
		config.QPS = float32(qps)
	}

	burstString, found := os.LookupEnv("NNF_REST_CONFIG_BURST")
	if found {
		burst, err := strconv.Atoi(burstString)
		if err != nil {
			setupLog.Error(err, "invalid value for NNF_REST_CONFIG_BURST")
			os.Exit(1)
		}
		config.Burst = burst
	}

	mgr, err := ctrl.NewManager(config, options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := dmCtrl.SetupReconcilers(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", dmCtrl.GetType())
		os.Exit(1)
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

type dataMovementControllerInterface interface {
	GetType() string
	SetOptions(*ctrl.Options)
	SetupReconcilers(manager.Manager) error
}

const (
	ManagerController = "manager"
	DefaultController = "default"
	NodeController    = "node"
)

func newDataMovementController(controller string) dataMovementControllerInterface {
	switch controller {
	case ManagerController:
		return &managerController{}
	case DefaultController:
		return &defaultController{}
	case NodeController:
		return &nodeController{}
	}

	setupLog.Info("unable to create controller", "controller", controller)
	os.Exit(1)
	return nil
}

// Manager Controller is that which manages various data movement systems - Secrets, Deployments, and DaemonSets
type managerController struct{}

func (*managerController) GetType() string { return ManagerController }
func (*managerController) SetOptions(opts *ctrl.Options) {
	namespaceCache := make(map[string]cache.Config)
	namespaceCache[corev1.NamespaceDefault] = cache.Config{}
	namespaceCache[nnfv1alpha5.DataMovementNamespace] = cache.Config{}
	opts.Cache = cache.Options{DefaultNamespaces: namespaceCache}
}

func (c *managerController) SetupReconcilers(mgr manager.Manager) (err error) {
	if err = (&controllers.NnfDataMovementManagerReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", c.GetType())
		os.Exit(1)
	}

	return
}

// Default Controller is that which operates on Data Movement resources.
type defaultController struct{}

func (*defaultController) GetType() string { return DefaultController }
func (*defaultController) SetOptions(opts *ctrl.Options) {
	namespaceCache := make(map[string]cache.Config)
	namespaceCache[corev1.NamespaceDefault] = cache.Config{}
	namespaceCache[nnfv1alpha5.DataMovementNamespace] = cache.Config{}
	namespaceCache[nnfv1alpha5.DataMovementProfileNamespace] = cache.Config{}
	opts.Cache = cache.Options{DefaultNamespaces: namespaceCache}
}

func (c *defaultController) SetupReconcilers(mgr manager.Manager) (err error) {
	if err = (&controllers.DataMovementReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		WatchNamespace: nnfv1alpha5.DataMovementNamespace,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", c.GetType())
		os.Exit(1)
	}

	return
}

// Node controller is that which operates on Data Movement resources to particular NNF Nodes
type nodeController struct {
	defaultController
}

func (*nodeController) GetType() string { return NodeController }
func (*nodeController) SetOptions(opts *ctrl.Options) {
	namespaceCache := make(map[string]cache.Config)
	namespaceCache[corev1.NamespaceDefault] = cache.Config{}
	namespaceCache[nnfv1alpha5.DataMovementNamespace] = cache.Config{}
	namespaceCache[nnfv1alpha5.DataMovementProfileNamespace] = cache.Config{}
	namespaceCache[os.Getenv("NNF_NODE_NAME")] = cache.Config{}
	opts.Cache = cache.Options{DefaultNamespaces: namespaceCache}
}

func (c *nodeController) SetupReconcilers(mgr manager.Manager) (err error) {
	if err = (&controllers.DataMovementReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		WatchNamespace: os.Getenv("NNF_NODE_NAME"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", c.GetType())
		os.Exit(1)
	}

	return
}

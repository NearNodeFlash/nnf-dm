/*
Copyright 2021.

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

package controllers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	lusv1alpha1 "github.hpe.com/hpe/hpc-rabsw-lustre-fs-operator/api/v1alpha1"
	_ "github.hpe.com/hpe/hpc-rabsw-lustre-fs-operator/config/crd/bases"

	nnfv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-sos/api/v1alpha1"
	_ "github.hpe.com/hpe/hpc-rabsw-nnf-sos/config/crd/bases"

	mpiv2beta1 "github.com/kubeflow/mpi-operator/v2/pkg/apis/kubeflow/v2beta1"

	dmv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-dm/api/v1alpha1"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	os.Setenv("ACK_GINKGO_DEPRECATIONS", "1.16.5")
	os.Setenv("X_DATA_MOVEMENT_TEST_ENV", "")

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{})
}

var _ = BeforeSuite(func() {
	logf.SetLogger(
		zap.New(zap.WriteTo(GinkgoWriter),
			zap.UseDevMode(true),
			zap.Level(zapcore.Level(-3))), // The inversion here is because zap is garbage and doesn't conform to any reasonable existing logging techniques
	)

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "vendor", "github.hpe.com", "hpe", "hpc-rabsw-lustre-fs-operator", "config", "crd", "bases"),
			filepath.Join("..", "vendor", "github.hpe.com", "hpe", "hpc-rabsw-nnf-sos", "config", "crd", "bases"),
			filepath.Join("..", "config", "crd", "bases"),
			filepath.Join("..", "config", "mpi")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = lusv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = nnfv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = mpiv2beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = dmv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	//k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	//Expect(err).NotTo(HaveOccurred())
	//Expect(k8sClient).NotTo(BeNil())

	// https://book.kubebuilder.io/cronjob-tutorial/writing-tests.html
	//    One thing that this autogenerated file is missing, however, is a way to actually start your controller.
	//    The code above will set up a client for interacting with your custom Kind, but will not be able to test
	//    your controller behavior. If you want to test your custom controller logic, you’ll need to add some familiar
	//    -looking manager logic to your BeforeSuite() function, so you can register your custom controller to
	//    run on this test cluster.

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	err = (&DataMovementReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = (&RsyncNodeDataMovementReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = (&RsyncTemplateReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	k8sClient = k8sManager.GetClient()
	Expect(k8sClient).NotTo(BeNil())

	go func() {
		defer GinkgoRecover()
		err := k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

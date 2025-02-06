/*
 * Copyright 2022-2025 Hewlett Packard Enterprise Development LP
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

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	lusv1beta1 "github.com/NearNodeFlash/lustre-fs-operator/api/v1beta1"
	nnfv1alpha5 "github.com/NearNodeFlash/nnf-sos/api/v1alpha5"
)

var _ = Describe("Data Movement Manager Test" /*Ordered, (Ginkgo v2)*/, func() {

	var lustre *lusv1beta1.LustreFileSystem
	var daemonset *appsv1.DaemonSet

	ns := &corev1.Namespace{}
	deployment := &appsv1.Deployment{}
	mgr := &nnfv1alpha5.NnfDataMovementManager{}
	labels := map[string]string{"control-plane": "controller-manager"}

	maxUnavailStr := "50%"
	maxSurgeStr := "0%"

	/* BeforeAll (Ginkgo v2)*/
	BeforeEach(func() {
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nnfv1alpha5.DataMovementNamespace,
			},
		}

		err := k8sClient.Create(ctx, ns)
		Expect(err == nil || errors.IsAlreadyExists(err)).Should(BeTrue())

		// Create a dummy deployment of the data movement manager
		deployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nnf-dm-manager-controller-manager",
				Namespace: nnfv1alpha5.DataMovementNamespace,
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: labels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: labels,
					},
					Spec: corev1.PodSpec{
						NodeSelector: labels,
						Containers: []corev1.Container{
							{
								Name:  "manager",
								Image: "controller:latest",
							},
						},
					},
				},
			},
		}

		err = k8sClient.Create(ctx, deployment)
		Expect(err == nil || errors.IsAlreadyExists(err)).Should(BeTrue())
	})

	BeforeEach(func() {
		maxUnavailable := intstr.FromString(maxUnavailStr)
		maxSurge := intstr.FromString(maxSurgeStr)
		mgr = &nnfv1alpha5.NnfDataMovementManager{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nnf-dm-manager-controller-manager",
				Namespace: nnfv1alpha5.DataMovementNamespace,
			},
			Spec: nnfv1alpha5.NnfDataMovementManagerSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: labels,
				},
				HostPath:  "/mnt/nnf",
				MountPath: "/mnt/nnf",
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "worker",
								Image: "controller:latest",
							},
						},
					},
				},
				UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
					Type: appsv1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDaemonSet{
						MaxUnavailable: &maxUnavailable,
						MaxSurge:       &maxSurge,
					},
				},
			},
		}

		daemonset = &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      daemonsetName,
				Namespace: mgr.Namespace,
			},
		}
	})

	JustBeforeEach(func() {
		Expect(k8sClient.Create(ctx, mgr)).Should(Succeed())
	})

	JustAfterEach(func() {
		Expect(k8sClient.Delete(ctx, mgr)).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(mgr), mgr)
		}).ShouldNot(Succeed())

		if lustre != nil {
			k8sClient.Delete(ctx, lustre) // may or may not be already deleted
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(lustre), lustre)
			}).ShouldNot(Succeed())
		}
	})

	It("Bootstraps all managed components", func() {
		Eventually(func(g Gomega) bool {
			g.Expect(fakeDSUpdates(daemonset, 2)).To(Succeed())
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mgr), mgr)).Should(Succeed())
			return mgr.Status.Ready
		}, "5s").Should(BeTrue())

		By("The updateStrategy appears in the daemon set")
		Eventually(func(g Gomega) error {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(daemonset), daemonset)).Should(Succeed())
			g.Expect(daemonset.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable.StrVal).Should(Equal(maxUnavailStr))
			g.Expect(daemonset.Spec.UpdateStrategy.RollingUpdate.MaxSurge.StrVal).Should(Equal(maxSurgeStr))
			return nil
		}).Should(Succeed())
	})

	It("Adds and removes global lustre volumes", func() {

		By("Wait for the manager to go ready")
		Eventually(func(g Gomega) bool {
			g.Expect(fakeDSUpdates(daemonset, 2)).To(Succeed())
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mgr), mgr)).Should(Succeed())
			return mgr.Status.Ready
		}).Should(BeTrue())

		By("Creating a Global Lustre File System")
		lustre = &lusv1beta1.LustreFileSystem{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "global",
				Namespace: corev1.NamespaceDefault,
			},
			Spec: lusv1beta1.LustreFileSystemSpec{
				Name:      "global",
				MgsNids:   "127.0.0.1@tcp",
				MountRoot: "/mnt/global",
			},
		}

		Expect(k8sClient.Create(ctx, lustre)).Should(Succeed())

		By("Status should not be ready")
		Eventually(func(g Gomega) bool {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mgr), mgr)).Should(Succeed())
			return mgr.Status.Ready
		}).Should(BeFalse())

		By("Expect namespace is added to lustre volume")
		Eventually(func(g Gomega) lusv1beta1.LustreFileSystemNamespaceSpec {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(lustre), lustre)).Should(Succeed())
			return lustre.Spec.Namespaces[mgr.Namespace]
		}).ShouldNot(BeNil())

		By("Expect finalizer is added to lustre volume")
		Eventually(func(g Gomega) []string {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(lustre), lustre)).Should(Succeed())
			return lustre.Finalizers
		}).Should(ContainElement(finalizer))

		By("The Volume appears in the daemon set")
		Eventually(func(g Gomega) error {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(daemonset), daemonset)).Should(Succeed())
			g.Expect(daemonset.Spec.Template.Spec.Volumes).Should(
				ContainElement(
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal(lustre.Name),
					}),
				),
			)
			g.Expect(daemonset.Spec.Template.Spec.Containers[0].VolumeMounts).Should(
				ContainElement(
					MatchFields(IgnoreExtras, Fields{
						"Name":      Equal(lustre.Name),
						"MountPath": Equal(lustre.Spec.MountRoot),
					}),
				),
			)
			return nil
		}).Should(Succeed())

		By("Status should be ready after daemonset is up to date")
		Eventually(func(g Gomega) bool {
			g.Expect(fakeDSUpdates(daemonset, 2)).To(Succeed())
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mgr), mgr)).Should(Succeed())
			return mgr.Status.Ready
		}).Should(BeTrue())

		By("Deleting Global Lustre File System")
		Expect(k8sClient.Delete(ctx, lustre)).To(Succeed())

		By("Status should be ready since daemonset was updated")
		Eventually(func(g Gomega) bool {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mgr), mgr)).Should(Succeed())
			return mgr.Status.Ready
		}).Should(BeFalse())

		By("Expect Global Lustre File system/finalizer to stay around until daemonset restarts pods without the volume")
		Eventually(func(g Gomega) error {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(daemonset), daemonset)).Should(Succeed())

			for _, v := range daemonset.Spec.Template.Spec.Volumes {
				desired := daemonset.Status.DesiredNumberScheduled
				updated := daemonset.Status.UpdatedNumberScheduled
				ready := daemonset.Status.NumberReady
				expectedGen := daemonset.ObjectMeta.Generation
				gen := daemonset.Status.ObservedGeneration

				// Fake the updates to the daemonset since the daemonset controller doesn't run
				g.Expect(fakeDSUpdates(daemonset, 2)).To(Succeed())

				if v.Name == lustre.Name {
					// If the volume still exists, then so should lustre + finalizer
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(lustre), lustre)).Should(Succeed())
					g.Expect(controllerutil.ContainsFinalizer(lustre, finalizer)).To(BeTrue())

				} else if gen != expectedGen && updated != desired && ready != desired {
					// If pods have not restarted, lustre + finalizer should still be there
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(lustre), lustre)).Should(Succeed())
					g.Expect(controllerutil.ContainsFinalizer(lustre, finalizer)).To(BeTrue())
				} else {
					// Once volume is gone and pods have restarted, lustre should be gone (and return error)
					return k8sClient.Get(ctx, client.ObjectKeyFromObject(lustre), lustre)
				}
			}

			return nil
		}, "15s").ShouldNot(Succeed())

		By("Status should be ready since daemonset is up to date from previous step")
		Eventually(func(g Gomega) bool {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mgr), mgr)).Should(Succeed())
			return mgr.Status.Ready
		}).Should(BeTrue())
	})

	It("Does not go ready when desired pods are 0", func() {
		By("staring ready with 4 desired pods")
		Eventually(func(g Gomega) bool {
			g.Expect(fakeDSUpdates(daemonset, 4)).To(Succeed())
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mgr), mgr)).Should(Succeed())
			return mgr.Status.Ready
		}, "5s").Should(BeTrue())

		By("changing to not ready when reduced to 0 desired pods")
		Eventually(func(g Gomega) bool {
			g.Expect(fakeDSUpdates(daemonset, 0)).To(Succeed())
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mgr), mgr)).Should(Succeed())
			return mgr.Status.Ready
		}, "5s").Should(BeFalse())
	})
})

// Envtest does not run the built-in controllers (e.g. daemonset controller).  This function fakes
// that out. Walk the counters up by one each time so we can exercise the controller watching these
// through a few iterations.
func fakeDSUpdates(ds *appsv1.DaemonSet, desired int32) error {

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ds), ds); err != nil {
			return err
		}
		ds.Status.DesiredNumberScheduled = desired

		ds.Status.ObservedGeneration++
		if ds.Status.ObservedGeneration > ds.ObjectMeta.Generation {
			ds.Status.ObservedGeneration = ds.ObjectMeta.Generation
		}

		ds.Status.UpdatedNumberScheduled++
		if ds.Status.UpdatedNumberScheduled > desired {
			ds.Status.UpdatedNumberScheduled = desired
		}

		ds.Status.NumberReady++
		if ds.Status.NumberReady > desired {
			ds.Status.NumberReady = desired
		}
		return k8sClient.Status().Update(ctx, ds)
	})

	return err

	// g.Expect(k8sClient.Status().Update(ctx, ds)).Should(Succeed())
}

/*
 * Copyright 2022 Hewlett Packard Enterprise Development LP
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

package controllers

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	lusv1alpha1 "github.com/NearNodeFlash/lustre-fs-operator/api/v1alpha1"
	dmv1alpha1 "github.com/NearNodeFlash/nnf-dm/api/v1alpha1"
)

var _ = Describe("Data Movement Manager Test", Ordered, func() {

	mgr := &dmv1alpha1.DataMovementManager{}
	labels := map[string]string{"control-plane": "controller-manager"}

	BeforeAll(func() {
		// Create a dummy deployment of the data movement manager
		deploy := appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "manager",
				Namespace: corev1.NamespaceDefault,
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

		Expect(k8sClient.Create(context.TODO(), &deploy)).Should(Succeed())
	})

	BeforeEach(func() {
		mgr = &dmv1alpha1.DataMovementManager{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "manager",
				Namespace: corev1.NamespaceDefault,
			},
			Spec: dmv1alpha1.DataMovementManagerSpec{
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
			},
		}
	})

	JustBeforeEach(func() {
		Expect(k8sClient.Create(context.TODO(), mgr)).Should(Succeed())
	})

	JustAfterEach(func() {
		Expect(k8sClient.Delete(context.TODO(), mgr)).Should(Succeed())
	})

	It("Bootstraps all managed components", func() {
		Eventually(func(g Gomega) bool {
			g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(mgr), mgr)).Should(Succeed())
			return mgr.Status.Ready
		}).Should(BeTrue())
	})

	It("Adds global lustre volumes", func() {

		By("Creating a Global Lustre File System")
		lustre := &lusv1alpha1.LustreFileSystem{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "global",
				Namespace: corev1.NamespaceDefault,
			},
			Spec: lusv1alpha1.LustreFileSystemSpec{
				Name:      "global",
				MgsNids:   "127.0.0.1@tcp",
				MountRoot: "/mnt/global",
			},
		}

		Expect(k8sClient.Create(context.TODO(), lustre)).Should(Succeed())

		By("The Volume appears in the daemon set")

		daemonset := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      daemonsetName,
				Namespace: mgr.Namespace,
			},
		}

		Eventually(func(g Gomega) error {
			g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(daemonset), daemonset)).Should(Succeed())

			g.Expect(daemonset.Spec.Template.Spec.Volumes).Should(
				ContainElement(
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal(lustre.Name),
					}),
				))

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

	})
})

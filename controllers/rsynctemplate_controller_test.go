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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	lusv1alpha1 "github.hpe.com/hpe/hpc-rabsw-lustre-fs-operator/api/v1alpha1"
	dmv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-dm/api/v1alpha1"
)

var _ = Describe("RsyncTemplate Controller", func() {
	var (
		key                types.NamespacedName
		created, retrieved *dmv1alpha1.RsyncTemplate
	)

	Context("Initialize", func() {

		key = types.NamespacedName{
			Name:      "rsync-template",
			Namespace: corev1.NamespaceDefault,
		}

		dskey := types.NamespacedName{
			Name:      key.Name + DaemonSetSuffix,
			Namespace: key.Namespace,
		}

		It("Template should initialize succcessfully", func() {

			created = &dmv1alpha1.RsyncTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: dmv1alpha1.RsyncTemplateSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"test-label": "true"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "hello-world",
								},
							},
						},
					},
				},
			}

			By("creating the rsync template")
			Expect(k8sClient.Create(context.TODO(), created)).To(Succeed())

			By("retrieving the rsync template")
			retrieved = &dmv1alpha1.RsyncTemplate{}
			Expect(k8sClient.Get(context.TODO(), key, retrieved)).To(Succeed())
		})

		It("DaemonSet should be present", func() {

			By("retrieving the daemon set")
			ds := &appsv1.DaemonSet{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), dskey, ds)
			}).Should(Succeed())
		})

		It("Should update with new lustre volume", func() {

			luskey := types.NamespacedName{
				Name:      "lustre-test",
				Namespace: corev1.NamespaceDefault,
			}

			By("creating a lustre file system")
			lus := &lusv1alpha1.LustreFileSystem{
				ObjectMeta: metav1.ObjectMeta{
					Name:      luskey.Name,
					Namespace: luskey.Namespace,
				},
				Spec: lusv1alpha1.LustreFileSystemSpec{
					Name:      "maui",
					MgsNid:    "localhost@tcp",
					MountRoot: "/lus/maui",
				},
			}
			Expect(k8sClient.Create(context.TODO(), lus)).To(Succeed())

			By("retrieving the lustre file system")
			lus = &lusv1alpha1.LustreFileSystem{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), luskey, lus)
			}).Should(Succeed())

			By("retrieving the updated daemon set")
			Eventually(func(g Gomega) []corev1.Volume {
				ds := &appsv1.DaemonSet{}
				g.Expect(k8sClient.Get(context.TODO(), dskey, ds)).To(Succeed())

				return ds.Spec.Template.Spec.Volumes

			}).Should(HaveLen(1))
		})

		It("Should delete successfully", func() {

			By("deleting the rsync template")
			Expect(k8sClient.Delete(context.TODO(), retrieved)).To(Succeed())

			// Matt says the garbage collector is not running on the test
			// environment. Need to confirm this on a real k8s setup.

			//By("dameon set should also be deleted")
			//ds := &appsv1.DaemonSet{}
			//Eventually(func() error {
			//	return k8sClient.Get(context.TODO(), dskey, ds)
			//}).ShouldNot(Succeed())
		})
	})
})

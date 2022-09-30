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
	"os"

	nnfv1alpha1 "github.com/NearNodeFlash/nnf-sos/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Data Movement Test" /*Ordered, (Ginkgo v2)*/, func() {
	var dm *nnfv1alpha1.NnfDataMovement = nil
	var srcPath, destPath string

	BeforeEach(func() {
		var err error

		srcPath, err = os.MkdirTemp("/tmp", "dm-test")
		Expect(err).ToNot(HaveOccurred())

		destPath, err = os.MkdirTemp("/tmp", "dm-test")
		Expect(err).ToNot(HaveOccurred())

		dm = &nnfv1alpha1.NnfDataMovement{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dm-test",
				Namespace: corev1.NamespaceDefault,
			},
			Spec: nnfv1alpha1.NnfDataMovementSpec{
				Source: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
					Path: srcPath,
				},
				Destination: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
					Path: destPath,
				},
				UserId:  uint32(os.Geteuid()),
				GroupId: uint32(os.Getegid()),
				Cancel:  false,
			},
		}
	})

	JustBeforeEach(func() {
		Expect(k8sClient.Create(context.TODO(), dm)).To(Succeed())
	})

	JustAfterEach(func() {
		Expect(k8sClient.Delete(context.TODO(), dm)).To(Succeed())
		Eventually(func() error {
			return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)
		}).ShouldNot(Succeed())
		Expect(os.Remove(srcPath)).To(Succeed())
		Expect(os.Remove(destPath)).To(Succeed())

	})

	Context("when a data movement operation succeeds", func() {
		It("should have a state and status of 'Finished' and 'Success'", func() {
			Eventually(func(g Gomega) nnfv1alpha1.NnfDataMovementStatus {
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				return dm.Status
			}, "3s").Should(MatchFields(IgnoreExtras, Fields{
				"State":  Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
				"Status": Equal(nnfv1alpha1.DataMovementConditionReasonSuccess),
			}))
		})
	})

	Context("when a data movement operation is cancelled", func() {
		It("should have a state and status of 'Finished' and 'Cancelled'", func() {
			By("ensuring the data movement started")
			Eventually(func(g Gomega) string {
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				return dm.Status.State
			}).Should(Equal(nnfv1alpha1.DataMovementConditionTypeRunning))

			By("setting the cancel flag to true")
			Eventually(func(g Gomega) error {
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				dm.Spec.Cancel = true
				return k8sClient.Update(context.TODO(), dm)
			}).Should(Succeed())

			By("verifying that it was cancelled successfully")
			Eventually(func(g Gomega) nnfv1alpha1.NnfDataMovementStatus {
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				return dm.Status
			}).Should(MatchFields(IgnoreExtras, Fields{
				"State":  Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
				"Status": Equal(nnfv1alpha1.DataMovementConditionReasonCancelled),
			}))
		})
	})
})

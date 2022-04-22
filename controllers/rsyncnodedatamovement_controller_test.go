/*
 * Copyright 2021, 2022 Hewlett Packard Enterprise Development LP
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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dmv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-sos/api/v1alpha1"
)

var _ = Describe("Rsync Node Data Movement Controller", func() {

	var rsync *dmv1alpha1.RsyncNodeDataMovement = nil

	BeforeEach(func() {
		rsync = &dmv1alpha1.RsyncNodeDataMovement{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rsync-node-test",
				Namespace: corev1.NamespaceDefault,
			},
			Spec: dmv1alpha1.RsyncNodeDataMovementSpec{
				Source:      "",
				Destination: "",
				UserId:      uint32(os.Geteuid()),
				GroupId:     uint32(os.Getegid()),
			},
		}
	})

	AfterEach(func() {
		// After each test specification, ensure the rsync resource is deleted and
		// that it is cleared from the client cache. This ensures the next test has
		// a clean slate.
		Eventually(func() error {
			Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(rsync), rsync)).To(Succeed())
			return k8sClient.Delete(context.TODO(), rsync)
		}).Should(Succeed())

		Eventually(func() error {
			return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(rsync), rsync)
		}).ShouldNot(Succeed())
	})

	BeforeEach(func() {
		_, err := os.Create("file.in")
		Expect(err).To(BeNil())

		Expect(os.Mkdir("directory", 0755)).To(Succeed())

		Expect(os.Mkdir("directory.in", 0755)).To(Succeed())
		_, err = os.Create("directory.in/file.in")
		Expect(err).To(BeNil())

		Expect(os.Mkdir("directory.out", 0755)).To(Succeed())
		Expect(os.Mkdir("directory/directory.out", 0755)).To(Succeed())

	})

	AfterEach(func() {
		Expect(os.Remove("file.in")).To(Succeed())
		if _, err := os.Stat("file.out"); !os.IsNotExist(err) {
			Expect(os.Remove("file.out")).To(Succeed())
		}

		Expect(os.RemoveAll("directory")).To(Succeed())
		Expect(os.RemoveAll("directory.in")).To(Succeed())
		Expect(os.RemoveAll("directory.out")).To(Succeed())
	})

	DescribeTable("Test various copy operations", func(source string, destination string, file bool) {
		rsync.Spec.Source = source
		rsync.Spec.Destination = destination

		By("Creating the rsync job")
		Expect(k8sClient.Create(context.TODO(), rsync)).To(Succeed())

		By("Ensuring the rsync job completes with success")
		Eventually(func() dmv1alpha1.RsyncNodeDataMovementStatus {
			Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(rsync), rsync)).To(Succeed())
			return rsync.Status
		}).Should(MatchFields(IgnoreExtras, Fields{
			"State":   Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
			"Status":  Equal(nnfv1alpha1.DataMovementConditionReasonSuccess),
			"Message": BeEmpty(),
		}))

		// Destinations ending with a ‘/’ mean “keep the name from source and place a copy of source under destination.
		if strings.HasSuffix(destination, "/") {
			destination = destination + source
		}

		if file {
			Expect(destination).To(BeARegularFile())
		} else {
			Expect(source).To(BeADirectory(), "Sanity check input source")
			Expect(source+"/file.in").To(BeARegularFile(), "Sanity check input source")

			Expect(destination + "/file.in").To(BeARegularFile())
		}

	},
		Entry("When source is file and destination is file", "file.in", "file.out", true),
		Entry("When source is file and destination is dir/", "file.in", "directory/", true),
		Entry("When source is file and destination is dir/file", "file.in", "directory/file.in", true),
		Entry("When source is file and destination is dir/file", "file.in", "directory/file.out", true),

		Entry("When source is dir/ and destination is dir", "directory.in/", "directory.out", false),
		Entry("When source is dir and destination is dir/", "directory.in", "directory.out/", false),
		Entry("When source is dir/ and destination is dir/dir", "directory.in/", "directory/directory.out", false),
		Entry("When source is dir and destination is dir/dir/", "directory.in", "directory/directory.out/", false),
	)
})

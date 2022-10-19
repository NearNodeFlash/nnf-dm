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
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	nnfv1alpha1 "github.com/NearNodeFlash/nnf-sos/api/v1alpha1"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// This is dumped into a temporary file and then ran as a bash script.
//
//go:embed fakeProgressOutput.sh
var progressOutputScript string

var _ = Describe("Data Movement Test" /*Ordered, (Ginkgo v2)*/, func() {
	var dm *nnfv1alpha1.NnfDataMovement
	var cm *corev1.ConfigMap
	var cmData = make(map[string]string)
	createCm := true
	var tmpDir string
	var srcPath string
	var destPath string
	dmCancel := false

	createFakeProgressScript := func() string {
		f, err := os.CreateTemp(tmpDir, "fakeProgressOutput-*.sh")
		Expect(err).ToNot(HaveOccurred())
		filePath := f.Name()

		bytes := []byte(progressOutputScript)
		_, err = f.Write(bytes)
		Expect(err).ToNot(HaveOccurred())

		err = os.Chmod(filePath, 0755)
		Expect(err).ToNot(HaveOccurred())

		return filePath
	}

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("/tmp", "dm-test")
		Expect(err).ToNot(HaveOccurred())

		srcPath = filepath.Join(tmpDir, "src")
		destPath = filepath.Join(tmpDir, "dest")

		// Set default values
		dmCancel = false
		createCm = true
		cmData[configMapKeyCmd] = "sleep 1"
		cmData[configMapKeyProgInterval] = "1s"
		cmData[configMapKeyDcpProgInterval] = "1"
		cmData[configMapKeyNumProcesses] = ""
		os.Unsetenv("NNF_NODE_NAME")
	})

	JustBeforeEach(func() {
		const testLabelKey = "dm-test"
		testLabel := fmt.Sprintf("%s-%s", testLabelKey, uuid.NewString())

		dm = &nnfv1alpha1.NnfDataMovement{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dm-test",
				Namespace: corev1.NamespaceDefault,
				Labels: map[string]string{
					testLabelKey: testLabel,
				},
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
				Cancel:  dmCancel,
			},
		}

		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: configMapNamespace,
				Labels: map[string]string{
					testLabelKey: testLabel,
				},
			},
			Data: cmData,
		}

		// Create CM and verify label
		if createCm {
			Expect(k8sClient.Create(context.TODO(), cm)).To(Succeed())
			Eventually(func(g Gomega) string {
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(cm), cm)).To(Succeed())
				return cm.Labels[testLabelKey]
			}).Should(Equal(testLabel))
		}

		// Create DM and verify label
		Expect(k8sClient.Create(context.TODO(), dm)).To(Succeed())
		Eventually(func(g Gomega) string {
			g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
			return dm.Labels[testLabelKey]
		}).Should(Equal(testLabel))

	})

	AfterEach(func() {
		// Remove datamovement
		k8sClient.Delete(context.TODO(), dm)
		Eventually(func() error {
			return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)
		}).ShouldNot(Succeed())

		// Remove configmap
		k8sClient.Delete(context.TODO(), cm)
		Eventually(func() error {
			return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(cm), cm)
		}).ShouldNot(Succeed())

		// Delete tmpdir and its contents
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
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

	Context("when the dm configmap has specified a dmCommand", func() {
		BeforeEach(func() {
			cmData[configMapKeyCmd] = "ls -l"
		})
		It("should use that command instead of the default mpirun", func() {
			Eventually(func(g Gomega) string {
				cmd := ""
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				if dm.Status.CommandStatus != nil {
					cmd = dm.Status.CommandStatus.Command
				}
				return cmd
			}).Should(Equal("/bin/" + cmData[configMapKeyCmd]))
		})
	})

	Context("when the dm config map has specified a valid dmNumProcesses and the dm namespace matches NNF_NODE_NAME", func() {
		BeforeEach(func() {
			cmData[configMapKeyCmd] = ""
			cmData[configMapKeyNumProcesses] = "17"
			os.Setenv("NNF_NODE_NAME", "default") // normally this isn't default, but we just want it to match
		})

		It("should use that number for the -np flag", func() {
			Eventually(func(g Gomega) string {
				cmd := ""
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				if dm.Status.CommandStatus != nil {
					cmd = dm.Status.CommandStatus.Command
				}
				return cmd
			}).Should(Equal(fmt.Sprintf(
				"mpirun --allow-run-as-root -np 17 --host localhost dcp --progress 1 %s %s", srcPath, destPath)))
		})
	})

	Context("when the dm config map has specified a valid dmNumProcesses and the dm namespace does not match NNF_NODE_NAME", func() {
		BeforeEach(func() {
			cmData[configMapKeyCmd] = ""
			cmData[configMapKeyNumProcesses] = "17"
		})

		It("should use len(hosts) (1) for the -np flag", func() {
			Eventually(func(g Gomega) string {
				cmd := ""
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				if dm.Status.CommandStatus != nil {
					cmd = dm.Status.CommandStatus.Command
				}
				return cmd
			}).Should(Equal(fmt.Sprintf(
				"mpirun --allow-run-as-root -np 1 --host localhost dcp --progress 1 %s %s", srcPath, destPath)))
		})
	})

	Context("when the dm config map has specified a invalid dmNumProcesses", func() {
		BeforeEach(func() {
			cmData[configMapKeyCmd] = ""
			cmData[configMapKeyNumProcesses] = "aa"
		})

		It("should use the default number for -np flag", func() {
			Eventually(func(g Gomega) string {
				cmd := ""
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				if dm.Status.CommandStatus != nil {
					cmd = dm.Status.CommandStatus.Command
				}
				return cmd
			}).Should(Equal(fmt.Sprintf(
				"mpirun --allow-run-as-root -np 1 --host localhost dcp --progress 1 %s %s", srcPath, destPath)))
		})
	})

	Context("when the dm config map has specified a valid dmProgressInterval", func() {
		BeforeEach(func() {
			cmData[configMapKeyProgInterval] = "25s"
		})

		It("parseConfigMapValues should return that value", func() {
			configMap := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: configMapNamespace}, configMap)).To(Succeed())
			_, dmProgressInterval, _, _ := parseConfigMapValues(*configMap)
			Expect(dmProgressInterval).To(Equal(25 * time.Second))
		})
	})

	Context("when the dm config map has specified a invalid dmProgressInterval", func() {
		BeforeEach(func() {
			cmData[configMapKeyProgInterval] = "aafdafdsa"
		})

		It("parseConfigMapValues should return the default value", func() {
			configMap := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: configMapNamespace}, configMap)).To(Succeed())
			_, dmProgressInterval, _, _ := parseConfigMapValues(*configMap)
			Expect(dmProgressInterval).To(Equal(configMapDefaultProgInterval))
		})
	})

	Context("when the dm config map has specified a empty dmProgressInterval", func() {
		BeforeEach(func() {
			cmData[configMapKeyProgInterval] = ""
		})

		It("parseConfigMapValues should return the default value", func() {
			configMap := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: configMapNamespace}, configMap)).To(Succeed())
			_, dmProgressInterval, _, _ := parseConfigMapValues(*configMap)
			Expect(dmProgressInterval).To(Equal(configMapDefaultProgInterval))
		})
	})

	Context("when the dm config map has specified a dmProgressInterval of less than 1s", func() {
		BeforeEach(func() {
			cmData[configMapKeyCmd] = "sleep .5"
			cmData[configMapKeyProgInterval] = "500ms"
		})

		It("the data movement should skip progress collection", func() {

			By("completing the data movement successfully")
			Eventually(func(g Gomega) nnfv1alpha1.NnfDataMovementStatus {
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				return dm.Status
			}, "3s").Should(MatchFields(IgnoreExtras, Fields{
				"State":  Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
				"Status": Equal(nnfv1alpha1.DataMovementConditionReasonSuccess),
			}))

			By("verify that CommandStatus is empty")
			Expect(dm.Status.CommandStatus.ProgressPercentage).To(BeNil())
			Expect(dm.Status.CommandStatus.LastMessage).To(BeEmpty())
		})
	})

	Context("when the dm config map has specified a valid dcpProgressInterval", func() {
		BeforeEach(func() {
			cmData[configMapKeyCmd] = ""
			cmData[configMapKeyDcpProgInterval] = "7.12s"
		})

		It("should use that number for the dcp --progress option after rounding", func() {
			Eventually(func(g Gomega) string {
				cmd := ""
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				if dm.Status.CommandStatus != nil {
					cmd = dm.Status.CommandStatus.Command
				}
				return cmd
			}).Should(Equal(fmt.Sprintf(
				"mpirun --allow-run-as-root -np 1 --host localhost dcp --progress 7 %s %s", srcPath, destPath)))
		})
	})

	Context("when the dm config map has specified an invalid dcpProgressInterval", func() {
		BeforeEach(func() {
			cmData[configMapKeyCmd] = ""
			cmData[configMapKeyDcpProgInterval] = "aaa"
		})

		It("should use the default number for the dcp --progress option", func() {
			Eventually(func(g Gomega) string {
				cmd := ""
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				if dm.Status.CommandStatus != nil {
					cmd = dm.Status.CommandStatus.Command
				}
				return cmd
			}).Should(Equal(fmt.Sprintf(
				"mpirun --allow-run-as-root -np 1 --host localhost dcp --progress 1 %s %s", srcPath, destPath)))
		})
	})

	Context("when the dm config map has specified an empty dcpProgressInterval", func() {
		BeforeEach(func() {
			cmData[configMapKeyCmd] = ""
			cmData[configMapKeyDcpProgInterval] = ""
		})

		It("should use the default number for the dcp --progress option", func() {
			Eventually(func(g Gomega) string {
				cmd := ""
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				if dm.Status.CommandStatus != nil {
					cmd = dm.Status.CommandStatus.Command
				}
				return cmd
			}).Should(Equal(fmt.Sprintf(
				"mpirun --allow-run-as-root -np 1 --host localhost dcp --progress 1 %s %s", srcPath, destPath)))
		})
	})

	Context("when there is no dm config map", func() {
		BeforeEach(func() {
			createCm = false
		})

		It("should requeue and not start running", func() {
			Consistently(func(g Gomega) string {
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				return dm.Status.State
			}).ShouldNot(Equal(nnfv1alpha1.DataMovementConditionTypeRunning))
		})
	})

	Context("when a data movement command fails", func() {
		BeforeEach(func() {
			cmData[configMapKeyCmd] = "false"
		})
		It("should have a State/Status of 'Finished'/'Failed'", func() {
			Eventually(func(g Gomega) nnfv1alpha1.NnfDataMovementStatus {
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				return dm.Status
			}).Should(MatchFields(IgnoreExtras, Fields{
				"State":  Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
				"Status": Equal(nnfv1alpha1.DataMovementConditionReasonFailed),
			}))
		})
	})

	Context("when a data movement is cancelled before it started", func() {
		finalizer := "dm-test"

		BeforeEach(func() {
			// Set cancel on creation so that data movement doesn't start
			dmCancel = true
			cmData[configMapKeyCmd] = "" // try to use mpirun dcp, it will fail
		})

		It("should have a State/Status of 'Finished'/'Cancelled' and StartTime/EndTime should be set to now", func() {
			nowMicro := metav1.NowMicro()

			By("Adding a finalizer to make sure the data movement stays around after delete")
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)
				dm.Spec.Cancel = true
				dm.Finalizers = append(dm.Finalizers, finalizer)
				return k8sClient.Update(context.TODO(), dm)
			})).To(Succeed())

			By("Attempting to delete the data movement so we get a DeletionTimestamp")
			Expect(k8sClient.Delete(context.TODO(), dm)).To(Succeed())
			Eventually(func(g Gomega) *metav1.Time {
				k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)
				return dm.DeletionTimestamp
			}, "10s").ShouldNot(BeNil())

			By("Checking the Status fields")
			Eventually(func(g Gomega) nnfv1alpha1.NnfDataMovementStatus {
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				return dm.Status
			}, "10s").Should(MatchFields(IgnoreExtras, Fields{
				"State":     Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
				"Status":    Equal(nnfv1alpha1.DataMovementConditionReasonCancelled),
				"StartTime": HaveField("Time", BeTemporally(">", nowMicro.Time)),
				"EndTime":   HaveField("Time", BeTemporally(">", nowMicro.Time)),
			}))
		})

		JustAfterEach(func() {
			// Remove the finalizer to ensure actual deletion
			Eventually(func(g Gomega) bool {
				k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)
				if controllerutil.ContainsFinalizer(dm, finalizer) {
					controllerutil.RemoveFinalizer(dm, finalizer)
				}
				k8sClient.Update(context.TODO(), dm)
				k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)
				return controllerutil.ContainsFinalizer(dm, finalizer)
			}).Should(BeFalse())
		})
	})

	Context("when a data movement operation is running", func() {
		var commandWithArgs string
		commandDuration := 5        // total duration of the fake output script
		commandIntervalInSec := 1.0 // interval to produce output

		BeforeEach(func() {
			scriptFilePath := createFakeProgressScript()
			_, err := os.Stat(scriptFilePath)
			Expect(err).ToNot(HaveOccurred())

			commandWithArgs = fmt.Sprintf("/bin/bash %s %d %f", scriptFilePath, commandDuration, commandIntervalInSec)
			cmData[configMapKeyCmd] = commandWithArgs
		})

		It("should update progress by parsing the output of the command", func() {
			startTime := metav1.NowMicro()

			Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(cm), cm)).To(Succeed())
			Expect(cm.Data[configMapKeyCmd]).To(Equal(commandWithArgs))

			By("ensuring that we do not have a progress to start")
			Eventually(func(g Gomega) *int32 {
				var progress *int32
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				if dm.Status.CommandStatus != nil {
					progress = dm.Status.CommandStatus.ProgressPercentage
				}
				return progress
			}).Should(BeNil())

			By("ensuring that we extract a progress value between 0 and 100 (i.e. 10)")
			Eventually(func(g Gomega) *int32 {
				var progress *int32
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				if dm.Status.CommandStatus != nil && dm.Status.CommandStatus.ProgressPercentage != nil {
					progress = dm.Status.CommandStatus.ProgressPercentage
				}
				return progress
			}, commandIntervalInSec*3).Should(SatisfyAll(
				HaveValue(BeNumerically(">", 0)),
				HaveValue(BeNumerically("<", 100)),
			))

			By("getting a progress of 100")
			Eventually(func(g Gomega) *int32 {
				var progress *int32
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				if dm.Status.CommandStatus != nil && dm.Status.CommandStatus.ProgressPercentage != nil {
					progress = dm.Status.CommandStatus.ProgressPercentage
				}
				return progress
			}, commandDuration).Should(HaveValue(Equal(int32(100))))

			endTime := metav1.NowMicro()
			elapsedTime := time.Since(startTime.Time)

			By("ensuring state/status of 'Finished'/'Success'")
			Eventually(func(g Gomega) nnfv1alpha1.NnfDataMovementStatus {
				g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				return dm.Status
			}, commandIntervalInSec*3).Should(MatchFields(IgnoreExtras, Fields{
				"State":  Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
				"Status": Equal(nnfv1alpha1.DataMovementConditionReasonSuccess),
			}))

			By("ensuring the LastMessage contains the 100% done output")
			Expect(dm.Status.CommandStatus.LastMessage).To(SatisfyAll(
				ContainSubstring("100%"),
				ContainSubstring("done"),
			))

			By("ensuring that the LastMessageTime is accurate")
			Expect(dm.Status.CommandStatus.LastMessageTime.Time).To(SatisfyAll(
				BeTemporally(">", startTime.Time),
				BeTemporally("<=", endTime.Time),
			))

			By("ensuring that ElaspedTime is accurate")
			Expect(dm.Status.CommandStatus.ElapsedTime.Duration).To(SatisfyAll(
				BeNumerically(">", commandDuration/2),
				BeNumerically("<=", elapsedTime),
			))

			GinkgoWriter.Printf("VERIFY: LastMessage: %s\n", dm.Status.CommandStatus.LastMessage)
			GinkgoWriter.Printf("VERIFY: LastMessageTime: %v\n", dm.Status.CommandStatus.LastMessageTime)
			GinkgoWriter.Printf("VERIFY: ElapsedTime: %v\n", dm.Status.CommandStatus.ElapsedTime)
		})
	})

	Context("when a data movement operation is cancelled", func() {
		BeforeEach(func() {
			cmData[configMapKeyCmd] = "sleep 5"
		})
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

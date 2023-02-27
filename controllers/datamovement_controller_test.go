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
	"strings"
	"time"

	"github.com/NearNodeFlash/nnf-dm/api/v1alpha1"
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

var _ = Describe("Data Movement Test", func() {

	Describe("Reconciler Tests", func() {
		var dm *nnfv1alpha1.NnfDataMovement
		var cm *corev1.ConfigMap
		createCm := true
		var tmpDir string
		var srcPath string
		var destPath string
		const testLabelKey = "dm-test"
		var testLabel string

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
			createCm = true
			testLabel = fmt.Sprintf("%s-%s", testLabelKey, uuid.NewString())

			tmpDir, err = os.MkdirTemp("/tmp", "dm-test")
			Expect(err).ToNot(HaveOccurred())

			srcPath = filepath.Join(tmpDir, "src")
			destPath = filepath.Join(tmpDir, "dest")

			// Ensure the DM namespace exists for the ConfigMap. Don't check the result because we
			// can only create this once. Since this is an unordered container, we cannot use
			// BeforeAll. Ignoring a Create() error is fine in this case.
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: v1alpha1.DataMovementNamespace,
				},
			}
			k8sClient.Create(context.TODO(), ns)

			cm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: v1alpha1.DataMovementNamespace,
					Labels: map[string]string{
						testLabelKey: testLabel,
					},
				},
				Data: map[string]string{
					// Use a command that will pass instead of using the default of mpirun - which will fail.
					// This is to ensure that our tests are less noisy on the output, as the mpirun will produce
					// an error in the output.
					configMapKeyCmd:             "true",
					configMapKeyProgInterval:    "1s",
					configMapKeyDcpProgInterval: "1s",
					configMapKeyNumProcesses:    "",
					configMapKeyMpiMaxSlots:     "",
					configMapKeyMpiSlots:        "",
				},
			}

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
					UserId:  0,
					GroupId: 0,
					Cancel:  false,
				},
			}
		})

		JustBeforeEach(func() {
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
			if dm != nil {
				Expect(k8sClient.Delete(context.TODO(), dm)).To(Succeed())
				Eventually(func() error {
					return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)
				}).ShouldNot(Succeed())
			}

			// Remove configmap
			if createCm {
				Expect(k8sClient.Delete(context.TODO(), cm)).To(Succeed())
				Eventually(func() error {
					return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(cm), cm)
				}).ShouldNot(Succeed())
			}

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
				cm.Data[configMapKeyCmd] = "/bin/ls -l"
			})
			It("should use that command instead of the default mpirun", func() {
				Eventually(func(g Gomega) string {
					cmd := ""
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					if dm.Status.CommandStatus != nil {
						cmd = dm.Status.CommandStatus.Command
					}
					return cmd
				}).Should(Equal(cm.Data[configMapKeyCmd]))
			})
		})

		Context("when the dm config map has specified a valid mpiNumProcesses and the dm namespace matches NNF_NODE_NAME", func() {
			BeforeEach(func() {
				cm.Data[configMapKeyCmd] = ""
				cm.Data[configMapKeyNumProcesses] = "17"
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
				}).Should(MatchRegexp(fmt.Sprintf(
					"mpirun --allow-run-as-root --hostfile (.+)/([^/]+) -np 17 dcp --progress 1 --uid 0 --gid 0 %s %s", srcPath, destPath)))
			})

			AfterEach(func() {
				os.Unsetenv("NNF_NODE_NAME")
			})
		})

		Context("when the dm config map has specified a valid mpiNumProcesses and the dm namespace does not match NNF_NODE_NAME", func() {
			BeforeEach(func() {
				cm.Data[configMapKeyCmd] = ""
				cm.Data[configMapKeyNumProcesses] = "17"
			})

			It("should use len(hosts) (1) for the -np flag", func() {
				Eventually(func(g Gomega) string {
					cmd := ""
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					if dm.Status.CommandStatus != nil {
						cmd = dm.Status.CommandStatus.Command
					}
					return cmd
				}).Should(MatchRegexp(fmt.Sprintf(
					"mpirun --allow-run-as-root --hostfile (.+)/([^/]+) -np 1 dcp --progress 1 --uid 0 --gid 0 %s %s", srcPath, destPath)))
			})
		})

		Context("when the dm config map has specified a invalid mpiNumProcesses", func() {
			BeforeEach(func() {
				cm.Data[configMapKeyCmd] = ""
				cm.Data[configMapKeyNumProcesses] = "aa"
			})

			It("should not use -np flag (default)", func() {
				Eventually(func(g Gomega) string {
					cmd := ""
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					if dm.Status.CommandStatus != nil {
						cmd = dm.Status.CommandStatus.Command
					}
					return cmd
				}).Should(MatchRegexp(fmt.Sprintf(
					"mpirun --allow-run-as-root --hostfile (.+)/([^/]+) dcp --progress 1 --uid 0 --gid 0 %s %s", srcPath, destPath)))
			})
		})

		Context("when the dm config map has specified an empty mpiNumProcesses", func() {
			BeforeEach(func() {
				cm.Data[configMapKeyCmd] = ""
				cm.Data[configMapKeyNumProcesses] = ""
			})

			It("should not use -np flag (default)", func() {
				Eventually(func(g Gomega) string {
					cmd := ""
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					if dm.Status.CommandStatus != nil {
						cmd = dm.Status.CommandStatus.Command
					}
					return cmd
				}).Should(MatchRegexp(fmt.Sprintf(
					"mpirun --allow-run-as-root --hostfile (.+)/([^/]+) dcp --progress 1 --uid 0 --gid 0 %s %s", srcPath, destPath)))
			})
		})

		DescribeTable("Parsing values from the dm config map for dmProgressInterval",
			func(durStr string, dur time.Duration) {
				configMap := &corev1.ConfigMap{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: configMapNamespace}, configMap)).To(Succeed())
				// For this test, we don't have a direct way to verify the progress interval, so use getCollectionInterval directly
				// instead of verifying full DM behavior.
				configMap.Data[configMapKeyProgInterval] = durStr
				dmProgressInterval := getCollectionInterval(configMap)
				Expect(dmProgressInterval).To(Equal(dur))
			},
			Entry("when dmProgressInterval is valid", "25s", 25*time.Second),
			Entry("when dmProgressInterval is invalid", "aafdafdsa", configMapDefaultProgInterval),
			Entry("when dmProgressInterval is empty", "", configMapDefaultProgInterval),
		)

		Context("when the dm config map has specified a dmProgressInterval of less than 1s", func() {
			BeforeEach(func() {
				cm.Data[configMapKeyCmd] = "sleep .5"
				cm.Data[configMapKeyProgInterval] = "500ms"
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
				cm.Data[configMapKeyCmd] = ""
				cm.Data[configMapKeyDcpProgInterval] = "7.12s"
			})

			It("should use that number for the dcp --progress option after rounding", func() {
				Eventually(func(g Gomega) string {
					cmd := ""
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					if dm.Status.CommandStatus != nil {
						cmd = dm.Status.CommandStatus.Command
					}
					return cmd
				}).Should(MatchRegexp(fmt.Sprintf(
					"mpirun --allow-run-as-root --hostfile (.+)/([^/]+) -np 1 dcp --progress 7 --uid 0 --gid 0 %s %s", srcPath, destPath)))
			})
		})

		Context("when the dm config map has specified an invalid dcpProgressInterval", func() {
			BeforeEach(func() {
				cm.Data[configMapKeyCmd] = ""
				cm.Data[configMapKeyDcpProgInterval] = "aaa"
			})

			It("should use the default number for the dcp --progress option", func() {
				Eventually(func(g Gomega) string {
					cmd := ""
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					if dm.Status.CommandStatus != nil {
						cmd = dm.Status.CommandStatus.Command
					}
					return cmd
				}).Should(MatchRegexp(fmt.Sprintf(
					"mpirun --allow-run-as-root --hostfile (.+)/([^/]+) -np 1 dcp --progress 1 --uid 0 --gid 0 %s %s", srcPath, destPath)))
			})
		})

		Context("when the dm config map has specified an empty dcpProgressInterval", func() {
			BeforeEach(func() {
				cm.Data[configMapKeyCmd] = ""
				cm.Data[configMapKeyDcpProgInterval] = ""
			})

			It("should use the default number for the dcp --progress option", func() {
				Eventually(func(g Gomega) string {
					cmd := ""
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					if dm.Status.CommandStatus != nil {
						cmd = dm.Status.CommandStatus.Command
					}
					return cmd
				}).Should(MatchRegexp(fmt.Sprintf(
					"mpirun --allow-run-as-root --hostfile (.+)/([^/]+) -np 1 dcp --progress 1 --uid 0 --gid 0 %s %s", srcPath, destPath)))
			})
		})

		Context("when the dm config map has specified extra mpi/dcp options", func() {
			BeforeEach(func() {
				cm.Data[configMapKeyCmd] = ""
				cm.Data[configMapKeyMpiOptions] = "--test1"
				cm.Data[configMapKeyDcpOptions] = "--test2"
			})

			It("should use the extra options for mpi and dcp", func() {
				Eventually(func(g Gomega) string {
					cmd := ""
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					if dm.Status.CommandStatus != nil {
						cmd = dm.Status.CommandStatus.Command
					}
					return cmd
				}).Should(MatchRegexp(fmt.Sprintf(
					"mpirun --allow-run-as-root --hostfile (.+)/([^/]+) -np 1 --test1 dcp --progress 1 --uid 0 --gid 0 --test2 %s %s", srcPath, destPath)))
			})
		})

		Context("when the dm config map has specified MPI slots", func() {
			BeforeEach(func() {
				cm.Data[configMapKeyCmd] = ""
				cm.Data[configMapKeyNumProcesses] = ""
				cm.Data[configMapKeyMpiSlots] = "4"
				cm.Data[configMapKeyMpiMaxSlots] = "8"
			})

			// There are more tests below for the slots/maxSlots in the hostfile itself, but this
			// verifies the integration between the config map, routine data movement, and the
			// contents being published.
			It("the MPI hostfile contents should use slots for each host", func() {
				Eventually(func(g Gomega) string {
					cmd := ""
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					if dm.Status.CommandStatus != nil {
						cmd = dm.Status.CommandStatus.Command
					}
					return cmd
				}).Should(MatchRegexp(fmt.Sprintf(
					"mpirun --allow-run-as-root --hostfile (.+)/([^/]+) dcp --progress 1 --uid 0 --gid 0 %s %s", srcPath, destPath)))

				Eventually(func(g Gomega) string {
					contents := ""
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					if dm.Status.CommandStatus != nil {
						contents = dm.Status.CommandStatus.MPIHostfileContents
					}
					return contents
				}).Should(Equal("localhost slots=4 max_slots=8\n"))
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
				cm.Data[configMapKeyCmd] = "false"
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
				dm.Spec.Cancel = true
				cm.Data[configMapKeyCmd] = "" // try to use mpirun dcp, it will fail
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

				dm = nil
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
				cm.Data[configMapKeyCmd] = commandWithArgs
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

		Context("when there are multiple lines of progress output to be read", func() {
			BeforeEach(func() {
				scriptFilePath := createFakeProgressScript()
				_, err := os.Stat(scriptFilePath)
				Expect(err).ToNot(HaveOccurred())

				cm.Data[configMapKeyCmd] = fmt.Sprintf("/bin/bash %s 10 .25", scriptFilePath)
				cm.Data[configMapKeyProgInterval] = "1s"
			})

			It("LastMessage should not include multiple lines of output", func() {
				Consistently(func(g Gomega) string {
					lastMessage := ""
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					if dm.Status.CommandStatus != nil {
						lastMessage = dm.Status.CommandStatus.LastMessage
					}
					return lastMessage
				}).ShouldNot(SatisfyAll(
					BeEmpty(),
					ContainSubstring("\n"),
				))
			})
		})

		Context("when a data movement operation is cancelled", func() {
			BeforeEach(func() {
				cm.Data[configMapKeyCmd] = "sleep 5"
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

		Context("when the data movement operation has a set uid and gid", func() {
			expectedGid := uint32(1000)
			expectedUid := uint32(2000)

			BeforeEach(func() {
				cm.Data[configMapKeyCmd] = ""
				dm.Spec.GroupId = expectedGid
				dm.Spec.UserId = expectedUid
			})
			It("should use those in the data movement command", func() {
				Eventually(func(g Gomega) string {
					cmd := ""
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					if dm.Status.CommandStatus != nil {
						cmd = dm.Status.CommandStatus.Command
					}
					return cmd
				}).Should(MatchRegexp(fmt.Sprintf(
					"mpirun --allow-run-as-root --hostfile (.+)/([^/]+) -np 1 dcp --progress 1 --uid %d --gid %d %s %s",
					expectedUid, expectedGid, srcPath, destPath)))
			})
		})
	})

	// These tests are for unit testing functions without the overhead of creating DM objects and allows
	// us to make better use of DescribeTable().
	Describe("Unit Tests (without Reconciler)", func() {

		Context("MPI hostfile creation", func() {
			hosts := []string{"node1", "node2"}
			DescribeTable("setting hosts, slots, and maxSlots",
				func(hosts []string, slots, maxSlots int, expected string) {
					hostfilePath, err := createMpiHostfile("my-dm", hosts, slots, maxSlots)
					defer os.RemoveAll(filepath.Dir(hostfilePath))
					Expect(err).To(BeNil())
					Expect(hostfilePath).ToNot(Equal(""))

					contents, err := os.ReadFile(hostfilePath)
					Expect(err).To(BeNil())
					Expect(string(contents)).To(Equal(expected))
				},
				Entry("with no slots or max_slots", hosts, -1, -1, "node1\nnode2\n"),
				Entry("with only slots", hosts, 4, -1, "node1 slots=4\nnode2 slots=4\n"),
				Entry("with only max_slots", hosts, -1, 4, "node1 max_slots=4\nnode2 max_slots=4\n"),
				Entry("with both slots and max_slots", hosts, 4, 4, "node1 slots=4 max_slots=4\nnode2 slots=4 max_slots=4\n"),
			)
		})

		Context("dmConfigMap Defaults", func() {
			DescribeTable("slot/maxSlot",
				func(slots, maxSlots string, expSlots, expMaxSlots int) {
					cm := &corev1.ConfigMap{
						Data: map[string]string{
							configMapKeyMpiSlots:    slots,
							configMapKeyMpiMaxSlots: maxSlots,
						},
					}
					s, m := getMpiSlots(cm)
					Expect(s).To(Equal(expSlots))
					Expect(m).To(Equal(expMaxSlots))

				},
				Entry("no values supplied - defaults used", "", "", configMapDefaultMpiSlots, configMapDefaultMpiMaxSlots),
				Entry("values supplied", "8", "16", 8, 16),
			)
		})

		Context("DM Command and Arguments", func() {
			DescribeTable("mpirun -np",
				func(ns string, np int, expected string) {
					dm := &nnfv1alpha1.NnfDataMovement{
						Spec: nnfv1alpha1.NnfDataMovementSpec{
							Source: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
								Path: "/src",
							},
							Destination: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
								Path: "/dest",
							},
							UserId:  9999,
							GroupId: 9999,
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: ns,
						},
					}
					numHosts := 10
					progressInt := 1
					mpiOpts := ""
					dcpOpts := ""
					cmd, args := getCmdAndArgs("", np, numHosts, "/tmp/xyz/hostfile", mpiOpts, progressInt, dcpOpts, dm)
					actual := fmt.Sprintf("%s %s", cmd, strings.Join(args, " "))
					Expect(actual).To(Equal(expected))
				},
				Entry("With -np", "", 8,
					"mpirun --allow-run-as-root --hostfile /tmp/xyz/hostfile -np 8 dcp --progress 1 --uid 9999 --gid 9999 /src /dest"),
				Entry("Without -np", "", -1,
					"mpirun --allow-run-as-root --hostfile /tmp/xyz/hostfile dcp --progress 1 --uid 9999 --gid 9999 /src /dest"),
				Entry("Lustre2Lustre (namespace != rabbit node) should use the len(hosts)", "fake-namespace", -1,
					"mpirun --allow-run-as-root --hostfile /tmp/xyz/hostfile -np 10 dcp --progress 1 --uid 9999 --gid 9999 /src /dest"),
			)

			DescribeTable("mpirun/dcp extra options",
				func(mpiOpts, dcpOpts, expected string) {
					dm := &nnfv1alpha1.NnfDataMovement{
						Spec: nnfv1alpha1.NnfDataMovementSpec{
							Source: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
								Path: "/src",
							},
							Destination: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
								Path: "/dest",
							},
							UserId:  9999,
							GroupId: 9999,
						},
					}
					np := 10
					numHosts := 10
					progressInt := 1
					cmd, args := getCmdAndArgs("", np, numHosts, "/tmp/xyz/hostfile", mpiOpts, progressInt, dcpOpts, dm)
					actual := fmt.Sprintf("%s %s", cmd, strings.Join(args, " "))
					Expect(actual).To(Equal(expected))
				},
				Entry("two mpi opts with args",
					"--extra x --opts y", "",
					"mpirun --allow-run-as-root --hostfile /tmp/xyz/hostfile -np 10 --extra x --opts y dcp --progress 1 --uid 9999 --gid 9999 /src /dest"),
				Entry("two dcp opts with args",
					"", "--more x --opts y",
					"mpirun --allow-run-as-root --hostfile /tmp/xyz/hostfile -np 10 dcp --progress 1 --uid 9999 --gid 9999 --more x --opts y /src /dest"),
				Entry("one mpi opt with no args",
					"--one", "",
					"mpirun --allow-run-as-root --hostfile /tmp/xyz/hostfile -np 10 --one dcp --progress 1 --uid 9999 --gid 9999 /src /dest"),
				Entry("one dcp opt with no args",
					"", "--two",
					"mpirun --allow-run-as-root --hostfile /tmp/xyz/hostfile -np 10 dcp --progress 1 --uid 9999 --gid 9999 --two /src /dest"),
				Entry("both mpi and dcp opts",
					"--one 1", "--two 2",
					"mpirun --allow-run-as-root --hostfile /tmp/xyz/hostfile -np 10 --one 1 dcp --progress 1 --uid 9999 --gid 9999 --two 2 /src /dest"),
			)
		})
	})
})

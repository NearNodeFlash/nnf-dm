/*
 * Copyright 2022-2023 Hewlett Packard Enterprise Development LP
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
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NearNodeFlash/nnf-sos/api/v1alpha1"
	nnfv1alpha1 "github.com/NearNodeFlash/nnf-sos/api/v1alpha1"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"go.openly.dev/pointy"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"
)

// This is dumped into a temporary file and then ran as a bash script.
//
//go:embed fakeProgressOutput.sh
var progressOutputScript string

var defaultCommand = "mpirun --allow-run-as-root --hostfile $HOSTFILE dcp --progress 1 --uid $UID --gid $GID $SRC $DEST"

var _ = Describe("Data Movement Test", func() {

	Describe("Reconciler Tests", func() {
		var dm *nnfv1alpha1.NnfDataMovement
		var cm *corev1.ConfigMap
		var dmCfg *dmConfig
		var dmCfgProfile dmConfigProfile
		createCm := true
		var tmpDir string
		var srcPath string
		var destPath string
		const testLabelKey = "dm-test"
		var testLabel string

		cmdBashPrefix := "/bin/bash -c "

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

			// Default config map data
			dmCfg = &dmConfig{
				Profiles: map[string]dmConfigProfile{
					nnfv1alpha1.DataMovementProfileDefault: {
						Command: defaultCommand,
					},
				},
				ProgressIntervalSeconds: 1,
			}
			dmCfgProfile = dmCfg.Profiles[nnfv1alpha1.DataMovementProfileDefault]

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
				// allow test to override the values in the default cfg profile
				dmCfg.Profiles[nnfv1alpha1.DataMovementProfileDefault] = dmCfgProfile

				// Convert the config to raw
				b, err := yaml.Marshal(dmCfg)
				Expect(err).ToNot(HaveOccurred())

				cm = &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName,
						Namespace: v1alpha1.DataMovementNamespace,
						Labels: map[string]string{
							testLabelKey: testLabel,
						},
					},
					Data: map[string]string{
						configMapKeyData: string(b),
					},
				}

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
			BeforeEach(func() {
				dmCfgProfile.Command = "sleep 1"
			})
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

		Context("when a data movement command has no progress output", func() {
			BeforeEach(func() {
				dmCfgProfile.Command = "sleep 1"
			})
			It("CommandStatus should not have a ProgressPercentage", func() {
				Eventually(func(g Gomega) nnfv1alpha1.NnfDataMovementStatus {
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					return dm.Status
				}, "3s").Should(MatchFields(IgnoreExtras, Fields{
					"State":  Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
					"Status": Equal(nnfv1alpha1.DataMovementConditionReasonSuccess),
				}))

				Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
				Expect(dm.Status.CommandStatus.ProgressPercentage).To(BeNil())
			})
		})

		Context("when the dm configmap has specified an overrideCmd", func() {
			BeforeEach(func() {
				dmCfgProfile.Command = "ls -l"
			})
			It("should use that command instead of the default mpirun", func() {
				Eventually(func(g Gomega) string {
					cmd := ""
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					if dm.Status.CommandStatus != nil {
						cmd = dm.Status.CommandStatus.Command
					}
					return cmd
				}).Should(Equal(cmdBashPrefix + dmCfgProfile.Command))
			})
		})

		Context("when the dm configmap does not have $HOSTFILE in the command", func() {
			BeforeEach(func() {
				dmCfgProfile.Command = "ls -l"
			})
			It("should use that command instead of the default mpirun", func() {
				Eventually(func(g Gomega) string {
					cmd := ""
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					if dm.Status.CommandStatus != nil {
						cmd = dm.Status.CommandStatus.Command
					}
					return cmd
				}).Should(Equal(cmdBashPrefix + dmCfgProfile.Command))
			})
		})

		Context("when the dm config map has specified a dmProgressInterval of less than 1s", func() {
			BeforeEach(func() {
				dmCfgProfile.Command = "sleep .5"
				dmCfg.ProgressIntervalSeconds = 0
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

		Context("when the dm config map has specified to store Stdout", func() {
			output := "this is not a test"
			BeforeEach(func() {
				dmCfgProfile.Command = "echo " + output
				dmCfgProfile.StoreStdout = true
			})

			It("should store the output in Status.Message", func() {

				By("completing the data movement successfully")
				Eventually(func(g Gomega) nnfv1alpha1.NnfDataMovementStatus {
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					return dm.Status
				}, "3s").Should(MatchFields(IgnoreExtras, Fields{
					"State":  Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
					"Status": Equal(nnfv1alpha1.DataMovementConditionReasonSuccess),
				}))

				By("verify that Message is equal to the output")
				Expect(dm.Status.Message).To(Equal(output + "\n"))
			})
		})

		Context("when the dm config map has specified to not store Stdout", func() {
			output := "this is not a test"
			BeforeEach(func() {
				dmCfgProfile.Command = "echo " + output
				dmCfgProfile.StoreStdout = false
			})

			It("should not store anything in Status.Message", func() {

				By("completing the data movement successfully")
				Eventually(func(g Gomega) nnfv1alpha1.NnfDataMovementStatus {
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					return dm.Status
				}, "3s").Should(MatchFields(IgnoreExtras, Fields{
					"State":  Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
					"Status": Equal(nnfv1alpha1.DataMovementConditionReasonSuccess),
				}))

				By("verify that Message is equal to the output")
				Expect(dm.Status.Message).To(BeEmpty())
			})
		})

		Context("when the UserConfig has specified to store Stdout", func() {
			output := "this is not a test"
			BeforeEach(func() {
				dmCfgProfile.Command = "echo " + output
				dm.Spec.UserConfig = &nnfv1alpha1.NnfDataMovementConfig{
					StoreStdout: true,
				}
			})

			It("should store the output in Status.Message", func() {

				By("completing the data movement successfully")
				Eventually(func(g Gomega) nnfv1alpha1.NnfDataMovementStatus {
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					return dm.Status
				}, "3s").Should(MatchFields(IgnoreExtras, Fields{
					"State":  Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
					"Status": Equal(nnfv1alpha1.DataMovementConditionReasonSuccess),
				}))

				By("verify that Message is equal to the output")
				Expect(dm.Status.Message).To(Equal(output + "\n"))
			})
		})

		Context("when the UserConfig has specified not to store Stdout", func() {
			output := "this is not a test"
			BeforeEach(func() {
				dmCfgProfile.Command = "echo " + output
				dm.Spec.UserConfig = &nnfv1alpha1.NnfDataMovementConfig{
					StoreStdout: false,
				}
			})

			It("should not store anything in Status.Message", func() {

				By("completing the data movement successfully")
				Eventually(func(g Gomega) nnfv1alpha1.NnfDataMovementStatus {
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					return dm.Status
				}, "3s").Should(MatchFields(IgnoreExtras, Fields{
					"State":  Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
					"Status": Equal(nnfv1alpha1.DataMovementConditionReasonSuccess),
				}))

				By("verify that Message is equal to the output")
				Expect(dm.Status.Message).To(BeEmpty())
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

		Context("when a non-default profile is supplied (and present)", func() {
			p := "test-profile"
			cmd := "sleep .1"

			BeforeEach(func() {
				dmCfgProfile = dmConfigProfile{
					Command: cmd,
				}
				dmCfg.Profiles[p] = dmCfgProfile
				dm.Spec.Profile = p
			})
			It("should use that profile to perform data movement", func() {

				By("completing the data movement successfully")
				Eventually(func(g Gomega) nnfv1alpha1.NnfDataMovementStatus {
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					return dm.Status
				}, "3s").Should(MatchFields(IgnoreExtras, Fields{
					"State":  Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
					"Status": Equal(nnfv1alpha1.DataMovementConditionReasonSuccess),
				}))

				By("verify that profile is used")
				Expect(dm.Spec.Profile).To(Equal(p))
				Expect(dm.Status.CommandStatus.Command).To(Equal(cmdBashPrefix + cmd))
			})
		})

		Context("when a non-default profile is supplied (and NOT present)", func() {
			m := "missing-test-profile"
			cmd := "sleep .1"

			BeforeEach(func() {
				dmCfgProfile = dmConfigProfile{
					Command: cmd,
				}
				dmCfg.Profiles["test-profile"] = dmCfgProfile
				dm.Spec.Profile = m
			})
			It("should use that profile to perform data movement and fail", func() {

				By("having a State/Status of 'Finished'/'Invalid'")
				Eventually(func(g Gomega) nnfv1alpha1.NnfDataMovementStatus {
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					return dm.Status
				}).Should(MatchFields(IgnoreExtras, Fields{
					"State":  Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
					"Status": Equal(nnfv1alpha1.DataMovementConditionReasonInvalid),
				}))

				By("verify that profile is used")
				Expect(dm.Spec.Profile).To(Equal(m))
			})
		})

		Context("when a data movement command fails", func() {
			BeforeEach(func() {
				dmCfgProfile.Command = "false"
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
				dmCfgProfile.Command = "false"
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
				dmCfgProfile.Command = commandWithArgs
			})

			It("should update progress by parsing the output of the command", func() {
				startTime := metav1.NowMicro()

				Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(cm), cm)).To(Succeed())
				Expect(dmCfgProfile.Command).To(Equal(commandWithArgs))

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

				dmCfgProfile.Command = fmt.Sprintf("/bin/bash %s 10 .25", scriptFilePath)
				dmCfg.ProgressIntervalSeconds = 1
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
				dmCfgProfile.Command = "sleep 5"
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
					"mpirun --allow-run-as-root --hostfile (.+)/([^/]+) dcp --progress 1 --uid %d --gid %d %s %s",
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
				Entry("with no slots or max_slots", hosts, 0, 0, "node1\nnode2\n"),
				Entry("with only slots", hosts, 4, 0, "node1 slots=4\nnode2 slots=4\n"),
				Entry("with only max_slots", hosts, 0, 4, "node1 max_slots=4\nnode2 max_slots=4\n"),
				Entry("with both slots and max_slots", hosts, 4, 4, "node1 slots=4 max_slots=4\nnode2 slots=4 max_slots=4\n"),
			)
		})

		Context("peekMpiHostfile", func() {
			It("should return the first line of the hostfile", func() {
				hosts := []string{"one", "two", "three"}
				slots, maxSlots := 16, 32
				hostfilePath, err := createMpiHostfile("my-dm", hosts, slots, maxSlots)
				Expect(err).ToNot(HaveOccurred())

				actual := peekMpiHostfile(hostfilePath)
				Expect(actual).To(Equal("one slots=16 max_slots=32\n"))
			})
		})

		Context("$HOSTFILE creation", func() {
			hosts := []string{"one", "two", "three"}
			dm := nnfv1alpha1.NnfDataMovement{
				Spec: nnfv1alpha1.NnfDataMovementSpec{
					UserId:  1000,
					GroupId: 2000,
					Source: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
						Path: "/src/",
					},
					Destination: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
						Path: "/dest/",
					},
				},
			}
			When("$HOSTFILE is present", func() {
				It("should create the hostfile", func() {
					profile := dmConfigProfile{
						Command: "mpirun --hostfile $HOSTFILE dcp src dest",
					}

					cmd, hostfile, err := buildDMCommand(context.TODO(), profile, hosts, &dm)
					Expect(err).ToNot(HaveOccurred())
					Expect(len(hostfile)).Should((BeNumerically(">", 0)))
					Expect(cmd).ToNot(BeEmpty())
					info, err := os.Stat(hostfile)
					Expect(err).ToNot(HaveOccurred())
					Expect(info).ToNot(BeNil())
				})

			})
			When("$HOSTFILE is not present", func() {
				It("should not create the hostfile", func() {
					profile := dmConfigProfile{
						Command: "mpirun -np 1 dcp src dest",
					}

					cmd, hostfile, err := buildDMCommand(context.TODO(), profile, hosts, &dm)
					Expect(err).ToNot(HaveOccurred())
					Expect(len(hostfile)).Should(Equal(0))
					Expect(cmd).ToNot(BeEmpty())
					info, err := os.Stat(hostfile)
					Expect(err).To(HaveOccurred())
					Expect(info).To(BeNil())
				})

			})
		})

		Context("User Config", func() {
			hosts := []string{"one", "two", "three"}
			expectedUid := 1000
			expectedGid := 2000
			srcPath := "/src/"
			destPath := "/dest/"
			dm := nnfv1alpha1.NnfDataMovement{
				Spec: nnfv1alpha1.NnfDataMovementSpec{
					UserId:  uint32(expectedUid),
					GroupId: uint32(expectedGid),
					Source: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
						Path: srcPath,
					},
					Destination: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
						Path: destPath,
					},
				},
			}

			When("DCPOptions are specified", func() {
				It("should inject the extra options before the $SRC argument to dcp", func() {
					profile := dmConfigProfile{
						Command: defaultCommand,
					}
					dm.Spec.UserConfig = &nnfv1alpha1.NnfDataMovementConfig{
						DCPOptions: "--extra opts",
					}
					expectedCmdRegex := fmt.Sprintf(
						"mpirun --allow-run-as-root --hostfile (.+)/([^/]+) dcp --progress 1 --uid %d --gid %d --extra opts %s %s",
						expectedUid, expectedGid, srcPath, destPath)

					cmd, _, err := buildDMCommand(context.TODO(), profile, hosts, &dm)
					Expect(err).ToNot(HaveOccurred())
					Expect(strings.Join(cmd, " ")).Should(MatchRegexp(expectedCmdRegex))
				})
			})

			When("slots/maxSlots are specified in the request", func() {
				DescribeTable("it should use the user slots vs the profile",
					func(numSlots *int) {
						profileSlots, profileMaxSlots := 3, 8

						profile := dmConfigProfile{
							Command:  defaultCommand,
							Slots:    profileSlots,
							MaxSlots: profileMaxSlots,
						}
						dm.Spec.UserConfig = &nnfv1alpha1.NnfDataMovementConfig{
							Slots:    numSlots,
							MaxSlots: numSlots,
						}
						_, hostfilePath, err := buildDMCommand(context.TODO(), profile, hosts, &dm)
						Expect(err).ToNot(HaveOccurred())
						Expect(hostfilePath).ToNot(BeEmpty())
						DeferCleanup(func() {
							Expect(os.Remove(hostfilePath)).ToNot(HaveOccurred())
						})

						content, err := os.ReadFile(hostfilePath)
						Expect(err).ToNot(HaveOccurred())
						Expect(string(content)).ToNot(BeEmpty())

						if numSlots == nil {
							// if nil, use the profile's slots
							Expect(string(content)).Should(MatchRegexp(fmt.Sprintf(" slots=%d", profileSlots)))
							Expect(string(content)).Should(MatchRegexp(fmt.Sprintf(" max_slots=%d", profileMaxSlots)))
						} else if *numSlots == 0 {
							// if 0, then don't use slots at all
							Expect(string(content)).ShouldNot(MatchRegexp(" slots"))
							Expect(string(content)).ShouldNot(MatchRegexp(" max_slots"))
						} else {
							Expect(string(content)).Should(MatchRegexp(fmt.Sprintf(" slots=%d", *numSlots)))
							Expect(string(content)).Should(MatchRegexp(fmt.Sprintf(" max_slots=%d", *numSlots)))
						}
					},
					Entry("when non-zero", pointy.Int(17)),
					Entry("when zero it should omit", pointy.Int(0)),
					Entry("when nil it should use the profile", nil),
				)
			})
		})

		Context("Destination mkdir path", func() {
			var tmpDir string

			setup := func(fileToMake string) {
				tmpDir = GinkgoT().TempDir()

				if fileToMake != "" {
					fileToMake = filepath.Join(tmpDir, fileToMake)
					Expect(os.MkdirAll(filepath.Dir(fileToMake), 0755)).To(Succeed())
					f, err := os.Create(fileToMake)
					Expect(err).ToNot(HaveOccurred())
					Expect(f.Close()).To(Succeed())
				}
			}

			// directory-directory OR directory-file
			// $DW_JOB_MY_GFS2 -> /lus/global/user/my-job/ = /lus/global/user/my-job/
			When("The source is a directory", func() {
				BeforeEach(func() {
					setup("")
				})
				It("should return the full destination directory", func() {
					srcPath := tmpDir
					destPath := "/lus/global/user/my-job"
					mkdirPath := "/lus/global/user/my-job"

					p, err := getDestinationDir(srcPath, destPath)
					Expect(err).ToNot(HaveOccurred())
					Expect(p).To(Equal(mkdirPath))
				})
			})

			// file-file
			// $DW_JOB_MY_GFS2/file.in -> /lus/global/user/my-job/file.out = /lus/global/user/my-job/
			When("When the source is a file and the destination suggests a file (no slash)", func() {
				BeforeEach(func() {
					setup("file.in")
				})

				It("should treat the destination as a file and return the directory of that file", func() {
					srcPath := filepath.Join(tmpDir, "file.in")
					destPath := "/lus/global/user/my-job/file.out"
					mkdirPath := "/lus/global/user/my-job"

					p, err := getDestinationDir(srcPath, destPath)
					Expect(err).ToNot(HaveOccurred())
					Expect(p).To(Equal(mkdirPath))
				})
			})

			// file-directory
			// $DW_JOB_MY_GFS2/file.in -> /lus/global/user/my-job/file.out/ = /lus/global/user/my-job/file.out/
			When("When the source is a file and the destination suggests a directory (slash)", func() {
				BeforeEach(func() {
					setup("file.in")
				})

				It("should return the full destination directory", func() {
					srcPath := filepath.Join(tmpDir, "file.in")
					destPath := "/lus/global/user/my-job/file.out/"
					mkdirPath := "/lus/global/user/my-job/file.out"

					p, err := getDestinationDir(srcPath, destPath)
					Expect(err).ToNot(HaveOccurred())
					Expect(p).To(Equal(mkdirPath))
				})
			})
		})
	})
})

/*
 * Copyright 2022-2024 Hewlett Packard Enterprise Development LP
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
	"reflect"
	"strings"
	"time"

	"github.com/NearNodeFlash/nnf-sos/api/v1alpha1"
	nnfv1alpha1 "github.com/NearNodeFlash/nnf-sos/api/v1alpha1"
	"github.com/go-logr/logr"
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
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// This is dumped into a temporary file and then ran as a bash script.
//
//go:embed fakeProgressOutput.sh
var progressOutputScript string

var defaultCommand = "mpirun --allow-run-as-root --hostfile $HOSTFILE dcp --progress 1 --uid $UID --gid $GID $SRC $DEST"

var _ = Describe("Data Movement Test", func() {

	Describe("Reconciler Tests", func() {
		var dm *nnfv1alpha1.NnfDataMovement
		var dmProfile *nnfv1alpha1.NnfDataMovementProfile
		createDmProfile := true
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
			createDmProfile = true
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

			// Default DM profile
			dmProfile = &nnfv1alpha1.NnfDataMovementProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default",
					Namespace: corev1.NamespaceDefault,
					Labels: map[string]string{
						testLabelKey: testLabel,
					},
				},
				Data: nnfv1alpha1.NnfDataMovementProfileData{
					Command:                 defaultCommand,
					ProgressIntervalSeconds: 1,
					Default:                 true,
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
					ProfileReference: corev1.ObjectReference{
						Kind:      reflect.TypeOf(nnfv1alpha1.NnfDataMovementProfile{}).Name(),
						Name:      dmProfile.Name,
						Namespace: dmProfile.Namespace,
					},
				},
			}
		})

		JustBeforeEach(func() {
			// Create DM Profile and verify label
			if createDmProfile {
				Expect(k8sClient.Create(context.TODO(), dmProfile)).To(Succeed())
				Eventually(func(g Gomega) string {
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dmProfile), dmProfile)).To(Succeed())
					return dmProfile.Labels[testLabelKey]
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
			if createDmProfile {
				Expect(k8sClient.Delete(context.TODO(), dmProfile)).To(Succeed())
				Eventually(func() error {
					return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dmProfile), dmProfile)
				}).ShouldNot(Succeed())
			}

			// Delete tmpdir and its contents
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		Context("when a data movement operation succeeds", func() {
			BeforeEach(func() {
				dmProfile.Data.Command = "sleep 1"
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
				dmProfile.Data.Command = "sleep 1"
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
				dmProfile.Data.Command = "ls -l"
			})
			It("should use that command instead of the default mpirun", func() {
				Eventually(func(g Gomega) string {
					cmd := ""
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					if dm.Status.CommandStatus != nil {
						cmd = dm.Status.CommandStatus.Command
					}
					return cmd
				}).Should(Equal(cmdBashPrefix + dmProfile.Data.Command))
			})
		})

		Context("when the dm configmap does not have $HOSTFILE in the command", func() {
			BeforeEach(func() {
				dmProfile.Data.Command = "ls -l"
			})
			It("should use that command instead of the default mpirun", func() {
				Eventually(func(g Gomega) string {
					cmd := ""
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					if dm.Status.CommandStatus != nil {
						cmd = dm.Status.CommandStatus.Command
					}
					return cmd
				}).Should(Equal(cmdBashPrefix + dmProfile.Data.Command))
			})
		})

		Context("when the dm config map has specified a dmProgressInterval of less than 1s", func() {
			BeforeEach(func() {
				dmProfile.Data.Command = "sleep .5"
				dmProfile.Data.ProgressIntervalSeconds = 0
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
				dmProfile.Data.Command = "echo " + output
				dmProfile.Data.StoreStdout = true
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
				dmProfile.Data.Command = "echo " + output
				dmProfile.Data.StoreStdout = false
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
				dmProfile.Data.Command = "echo " + output
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
				dmProfile.Data.Command = "echo " + output
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
				createDmProfile = false
			})

			It("should requeue and not start running", func() {
				Consistently(func(g Gomega) string {
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					return dm.Status.State
				}).ShouldNot(Equal(nnfv1alpha1.DataMovementConditionTypeRunning))
			})
		})

		Context("when a non-default profile is supplied (and present)", func() {
			p := &nnfv1alpha1.NnfDataMovementProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-profile",
					Namespace: corev1.NamespaceDefault,
				},
			}
			cmd := "sleep .1"

			BeforeEach(func() {
				p.Data.Command = cmd
				dm.Spec.ProfileReference = corev1.ObjectReference{
					Kind:      reflect.TypeOf(nnfv1alpha1.NnfDataMovementProfile{}).Name(),
					Name:      p.Name,
					Namespace: p.Namespace,
				}

				Expect(k8sClient.Create(context.TODO(), p)).To(Succeed(), "create nnfDataMovementProfile")

				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(p), p)).To(Succeed())
				}, "3s", "1s").Should(Succeed(), "wait for create of NnfDataMovementProfile")
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
				Expect(dm.Spec.ProfileReference).To(MatchFields(IgnoreExtras,
					Fields{
						"Kind":      Equal(reflect.TypeOf(nnfv1alpha1.NnfDataMovementProfile{}).Name()),
						"Name":      Equal(p.Name),
						"Namespace": Equal(p.Namespace),
					},
				))
				Expect(dm.Status.CommandStatus.Command).To(Equal(cmdBashPrefix + cmd))
			})
		})

		Context("when a non-default profile is supplied (and NOT present)", func() {
			m := &nnfv1alpha1.NnfDataMovementProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-test-profile",
					Namespace: corev1.NamespaceDefault,
				},
			}
			cmd := "sleep .1"

			BeforeEach(func() {
				m.Data.Command = cmd
				dm.Spec.ProfileReference = corev1.ObjectReference{
					Kind:      reflect.TypeOf(nnfv1alpha1.NnfDataMovementProfile{}).Name(),
					Name:      m.Name,
					Namespace: m.Namespace,
				}
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
				Expect(dm.Spec.ProfileReference).To(MatchFields(IgnoreExtras,
					Fields{
						"Kind":      Equal(reflect.TypeOf(nnfv1alpha1.NnfDataMovementProfile{}).Name()),
						"Name":      Equal(m.Name),
						"Namespace": Equal(m.Namespace),
					},
				))
			})
		})

		Context("when a data movement command fails", func() {
			BeforeEach(func() {
				dmProfile.Data.Command = "false"
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
				dmProfile.Data.Command = "false"
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
				dmProfile.Data.Command = commandWithArgs
			})

			It("should update progress by parsing the output of the command", func() {
				startTime := metav1.NowMicro()

				Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dmProfile), dmProfile)).To(Succeed())
				Expect(dmProfile.Data.Command).To(Equal(commandWithArgs))

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

				dmProfile.Data.Command = fmt.Sprintf("/bin/bash %s 10 .25", scriptFilePath)
				dmProfile.Data.ProgressIntervalSeconds = 1
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
				dmProfile.Data.Command = "sleep 5"
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
					hostfilePath, err := writeMpiHostfile("my-dm", hosts, slots, maxSlots)
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
				hostfilePath, err := writeMpiHostfile("my-dm", hosts, slots, maxSlots)
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
					profile := nnfv1alpha1.NnfDataMovementProfile{}
					profile.Data.Command = "mpirun --hostfile $HOSTFILE dcp src dest"

					hostfile, err := createMpiHostfile(&profile, hosts, &dm)
					Expect(err).ToNot(HaveOccurred())
					Expect(len(hostfile)).Should((BeNumerically(">", 0)))
					info, err := os.Stat(hostfile)
					Expect(err).ToNot(HaveOccurred())
					Expect(info).ToNot(BeNil())
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
					profile := nnfv1alpha1.NnfDataMovementProfile{}
					profile.Data.Command = defaultCommand

					dm.Spec.UserConfig = &nnfv1alpha1.NnfDataMovementConfig{
						DCPOptions: "--extra opts",
					}
					expectedCmdRegex := fmt.Sprintf(
						"mpirun --allow-run-as-root --hostfile /tmp/hostfile dcp --progress 1 --uid %d --gid %d --extra opts %s %s",
						expectedUid, expectedGid, srcPath, destPath)

					cmd, err := buildDMCommand(&profile, "/tmp/hostfile", &dm, log.FromContext(context.TODO()))
					Expect(err).ToNot(HaveOccurred())
					Expect(strings.Join(cmd, " ")).Should(MatchRegexp(expectedCmdRegex))
				})
			})

			When("slots/maxSlots are specified in the request", func() {
				DescribeTable("it should use the user slots vs the profile",
					func(numSlots *int) {
						profileSlots, profileMaxSlots := 3, 8

						profile := nnfv1alpha1.NnfDataMovementProfile{
							Data: nnfv1alpha1.NnfDataMovementProfileData{
								Command:  defaultCommand,
								Slots:    profileSlots,
								MaxSlots: profileMaxSlots,
							},
						}
						dm.Spec.UserConfig = &nnfv1alpha1.NnfDataMovementConfig{
							Slots:    numSlots,
							MaxSlots: numSlots,
						}

						hostfilePath, err := createMpiHostfile(&profile, hosts, &dm)
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

		Context("getDestinationDir", func() {
			expectedSourceFile := "/src/job/data.out"
			destRoot := "/lus/global/user"

			DescribeTable("",
				func(src, dest, expected string) {
					tmpDir := GinkgoT().TempDir()

					// create sourceFile in tmpdir root
					sourceFilePath := filepath.Join(tmpDir, expectedSourceFile)
					Expect(os.MkdirAll(filepath.Dir(sourceFilePath), 0755)).To(Succeed())
					f, err := os.Create(sourceFilePath)
					Expect(err).ToNot(HaveOccurred())
					Expect(f.Close()).To(Succeed())

					// create destDir
					destDirPath := filepath.Join(tmpDir, destRoot)
					Expect(os.MkdirAll(destDirPath, 0755)).To(Succeed())

					// for this one case, we want the destination file to exist
					if dest == "/lus/global/user/data.out" {
						existing := filepath.Join(tmpDir, dest)
						f, err := os.Create(existing)
						Expect(err).ToNot(HaveOccurred())
						Expect(f.Close()).To(Succeed())
					}

					// Replace the paths to use tmpDir root
					newSrc := strings.Replace(src, "$DW_JOB_workflow", filepath.Join(tmpDir, "src"), -1)
					newDest := filepath.Join(tmpDir, dest)
					// don't drop trailing slashes on the dest
					if strings.HasSuffix(dest, "/") {
						newDest += "/"
					}

					dm := &nnfv1alpha1.NnfDataMovement{
						Spec: nnfv1alpha1.NnfDataMovementSpec{
							Source: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
								Path: newSrc,
							},
							Destination: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
								Path: newDest,
							},
							UserId:  1234,
							GroupId: 2345,
						},
					}

					dmProfile := &nnfv1alpha1.NnfDataMovementProfile{}

					destDir, err := getDestinationDir(dmProfile, dm, "", logr.Logger{})
					destDir = strings.Replace(destDir, tmpDir, "", -1) // remove tmpdir from the path
					Expect(err).ToNot(HaveOccurred())
					Expect(destDir).To(Equal(expected))
				},
				Entry("file-dir", "$DW_JOB_workflow/job/data.out", "/lus/global/user", "/lus/global/user"),
				Entry("file-dir/", "$DW_JOB_workflow/job/data.out", "/lus/global/user/", "/lus/global/user"),
				Entry("file-file", "$DW_JOB_workflow/job/data.out", "/lus/global/user/data.out", "/lus/global/user"),
				Entry("file-DNE file", "$DW_JOB_workflow/job/data.out", "/lus/global/user/idontexist", "/lus/global/user"),
				Entry("file-DNE dir", "$DW_JOB_workflow/job/data.out", "/lus/global/user/newdir/", "/lus/global/user/newdir"),
				Entry("file-DNE dir/file", "$DW_JOB_workflow/job/data.out", "/lus/global/user/newdir/idontexist", "/lus/global/user/newdir"),
				Entry("file-DNE dir/dir", "$DW_JOB_workflow/job/data.out", "/lus/global/user/newdir/newdir2/", "/lus/global/user/newdir/newdir2"),
				Entry("file-DNE dir/dir/file", "$DW_JOB_workflow/job/data.out", "/lus/global/user/newdir/newdir2/idontexist", "/lus/global/user/newdir/newdir2"),

				Entry("dir-dir", "$DW_JOB_workflow/job", "/lus/global/user", "/lus/global/user"),
				Entry("dir-dir/", "$DW_JOB_workflow/job", "/lus/global/user/", "/lus/global/user"),
				Entry("dir/-dir", "$DW_JOB_workflow/job/", "/lus/global/user", "/lus/global/user"),
				Entry("dir/-dir/", "$DW_JOB_workflow/job/", "/lus/global/user/", "/lus/global/user"),
				Entry("dir-DNE dir", "$DW_JOB_workflow/job", "/lus/global/user/newdir", "/lus/global/user/newdir"),
				Entry("dir-DNE dir/", "$DW_JOB_workflow/job", "/lus/global/user/newdir/", "/lus/global/user/newdir"),
				Entry("dir/-DNE dir", "$DW_JOB_workflow/job/", "/lus/global/user/newdir", "/lus/global/user/newdir"),
				Entry("dir/-DNE dir/", "$DW_JOB_workflow/job/", "/lus/global/user/newdir/", "/lus/global/user/newdir"),
				Entry("dir-DNE dir/dir", "$DW_JOB_workflow/job", "/lus/global/user/newdir/newdir2", "/lus/global/user/newdir/newdir2"),
				Entry("dir-DNE dir/dir/", "$DW_JOB_workflow/job", "/lus/global/user/newdir/newdir2/", "/lus/global/user/newdir/newdir2"),
				Entry("dir/-DNE dir/dir", "$DW_JOB_workflow/job/", "/lus/global/user/newdir/newdir2", "/lus/global/user/newdir/newdir2"),
				Entry("dir/-DNE dir/dir/", "$DW_JOB_workflow/job/", "/lus/global/user/newdir/newdir2/", "/lus/global/user/newdir/newdir2"),

				Entry("root-dir", "$DW_JOB_workflow", "/lus/global/user", "/lus/global/user"),
				Entry("root-dir/", "$DW_JOB_workflow", "/lus/global/user/", "/lus/global/user"),
				Entry("root/-dir", "$DW_JOB_workflow/", "/lus/global/user", "/lus/global/user"),
				Entry("root/-dir/", "$DW_JOB_workflow/", "/lus/global/user/", "/lus/global/user"),
				Entry("root-DNE dir", "$DW_JOB_workflow", "/lus/global/user/newdir", "/lus/global/user/newdir"),
				Entry("root-DNE dir/", "$DW_JOB_workflow", "/lus/global/user/newdir/", "/lus/global/user/newdir"),
				Entry("root/-DNE dir", "$DW_JOB_workflow/", "/lus/global/user/newdir", "/lus/global/user/newdir"),
				Entry("root/-DNE dir/", "$DW_JOB_workflow/", "/lus/global/user/newdir/", "/lus/global/user/newdir"),
			)
		})

		Context("extractIndexMountDir", func() {
			ns := "winchell31"
			DescribeTable("",
				func(path, expected string, expectError bool) {

					idxMount, err := extractIndexMountDir(path, ns)
					Expect(idxMount).To(Equal(expected))
					if expectError {
						Expect(err).To(HaveOccurred())
					} else {
						Expect(err).To(Not(HaveOccurred()))
					}
				},
				Entry("standard 1", "/mnt/nnf/12345-0/winchell31-0/dir1", "winchell31-0", false),
				Entry("standard 2", "/mnt/nnf/12345-0/winchell31-7/path/to/a/file", "winchell31-7", false),

				// root is a special case where you would "double up", we don't want to return an
				// index mount in this case since it's already there.
				Entry("root", "/mnt/nnf/12345-0/winchell31-0", "", false),
				Entry("root with trailing slash", "/mnt/nnf/12345-0/winchell31-11/", "winchell31-11", false),
				Entry("really big index", "/mnt/nnf/12345-0/winchell31-9999999999999/", "winchell31-9999999999999", false),
				Entry("empty", "", "", true),

				Entry("cannot extract - wrong namespace", "/mnt/nnf/12345-0/wrong-namespace-0/", "", true),
				Entry("cannot extract - extra '-digit'", "mnt/nnf/12345-0/winchell31-0-0/", "", true),
			)
		})

		Context("handleIndexMountDir", func() {
			idxMount := "rabbit-node-2-10"
			expectedSourceFile := fmt.Sprintf("/%s/job/data.out", idxMount)
			destRoot := "/lus/global/user"

			DescribeTable("",
				func(src, dest, destDir, expectedDir, expectedPath string) {
					tmpDir := GinkgoT().TempDir()

					// create sourceFile in tmpdir root
					sourceFilePath := filepath.Join(tmpDir, expectedSourceFile)
					Expect(os.MkdirAll(filepath.Dir(sourceFilePath), 0755)).To(Succeed())
					f, err := os.Create(sourceFilePath)
					Expect(err).ToNot(HaveOccurred())
					Expect(f.Close()).To(Succeed())

					// create destDir
					destDirPath := filepath.Join(tmpDir, destRoot)
					Expect(os.MkdirAll(destDirPath, 0755)).To(Succeed())

					// Replace the paths to use tmpDir root
					newSrc := strings.Replace(src, "$DW_JOB_workflow", filepath.Join(tmpDir, "rabbit-node-2-10"), -1)
					newDest := filepath.Join(tmpDir, dest)
					// don't drop trailing slashes on the dest
					if strings.HasSuffix(dest, "/") {
						newDest += "/"
					}

					// We need a DM to stuff the paths and check the updated destination after account for index mount
					dm := &nnfv1alpha1.NnfDataMovement{
						Spec: nnfv1alpha1.NnfDataMovementSpec{
							Source: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
								Path: newSrc,
							},
							Destination: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
								Path: newDest,
							},
						},
					}

					dmProfile := &nnfv1alpha1.NnfDataMovementProfile{}

					newDestDir, err := handleIndexMountDir(dmProfile, dm, destDir, idxMount, "", logr.Logger{})
					Expect(err).ToNot((HaveOccurred()))

					// Remove any tmpDir paths before verifying
					newDestDir = strings.Replace(newDestDir, tmpDir, "", -1)
					dm.Spec.Destination.Path = strings.Replace(dm.Spec.Destination.Path, tmpDir, "", -1)
					Expect(newDestDir).To(Equal(expectedDir), "updated dest directory")
					Expect(dm.Spec.Destination.Path).To(Equal(expectedPath), "updated DM destination path")
				},

				// Entry("","src", "dest", "destDir", "idxMount", "expectedDir", "expectedPath" ),
				Entry("file-dir", "$DW_JOB_workflow/job/data.out", "/lus/global/user/", "/lus/global/user", "/lus/global/user/rabbit-node-2-10", "/lus/global/user/rabbit-node-2-10/"),
				Entry("file-file", "$DW_JOB_workflow/job/data.out", "/lus/global/user/newname.out", "/lus/global/user", "/lus/global/user/rabbit-node-2-10", "/lus/global/user/rabbit-node-2-10/newname.out"),

				Entry("dir-dir", "$DW_JOB_workflow/job/", "/lus/global/user/", "/lus/global/user", "/lus/global/user/rabbit-node-2-10", "/lus/global/user/rabbit-node-2-10/"),
				Entry("dir-file", "$DW_JOB_workflow/job/", "/lus/global/user/newdir", "/lus/global/user/newdir", "/lus/global/user/newdir/rabbit-node-2-10", "/lus/global/user/newdir/rabbit-node-2-10"),

				Entry("root-dir", "$DW_JOB_workflow", "/lus/global/user/", "/lus/global/user", "/lus/global/user", "/lus/global/user/"),
				Entry("root/-dir", "$DW_JOB_workflow/", "/lus/global/user/", "/lus/global/user", "/lus/global/user/rabbit-node-2-10", "/lus/global/user/rabbit-node-2-10/"),
			)
		})
	})
})

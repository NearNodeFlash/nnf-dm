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
	"reflect"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mpiv2beta1 "github.com/kubeflow/mpi-operator/v2/pkg/apis/kubeflow/v2beta1"

	dwsv1alpha1 "github.com/HewlettPackard/dws/api/v1alpha1"
	lusv1alpha1 "github.com/NearNodeFlash/lustre-fs-operator/api/v1alpha1"
	dmv1alpha1 "github.com/NearNodeFlash/nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.com/NearNodeFlash/nnf-sos/api/v1alpha1"
)

const (
	testNumberOfNodes = 2
	orchestratedMGS   = "172.0.0.1@tcp"
	globalMGS         = "172.0.0.2@tcp"

	// For our tests, when we orchestate with an external MGS, we'll use the global filesystem's MGS.
	externalMGS = globalMGS
)

var _ = Describe("Data Movement Controller", func() {

	var (
		nodeKeys          []types.NamespacedName
		storageKey, dmKey types.NamespacedName
		storage           *nnfv1alpha1.NnfStorage
		access            *nnfv1alpha1.NnfAccess
		dm                *nnfv1alpha1.NnfDataMovement
		setup             sync.Once
		lustre            *lusv1alpha1.LustreFileSystem
		fsName            string
	)

	BeforeEach(func() {

		// Do once a bunch of setup for this class of testing. This creates
		// the nnf-dm-system namespace and 2 rabbit nodes with namespaces so
		// rsync jobs can be assigned.
		//
		// Note: Unfortunately deleting these namespaces doesn't seem to work
		// in envtest; they forever sit around with finalizers even though -
		// to my knowledge - there are no dependencies remaining. So this means
		// that all testing that occurs after this block will have these resources
		// present, even if undesired. Keep this in mind when writing new tests.
		By("setup once: namespaces and nodes")
		setup.Do(func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nnf-dm-system",
				},
			}

			Expect(k8sClient.Create(context.TODO(), ns)).To(Succeed())

			// Create some nodes for testing on
			nodeKeys = []types.NamespacedName{
				{Name: "test-node-0"},
				{Name: "test-node-1"},
			}

			for _, nodeKey := range nodeKeys {
				ns = &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: nodeKey.Name,
					},
				}

				Expect(k8sClient.Create(context.TODO(), ns)).To(Succeed())

				node := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: nodeKey.Name,
						Labels: map[string]string{
							"cray.nnf.node": "true",
						},
					},
				}

				Expect(k8sClient.Create(context.TODO(), node)).To(Succeed())
			}

		})
	})

	// Before each test ensure there is a Node with the proper label (cray.nnf.node=true), and
	// there is NNF Storage that contains that node as its one and only allocation.
	BeforeEach(func() {
		By("Define NnfStorage and NnfAccess")

		storageKey = types.NamespacedName{
			Name:      "test-storage",
			Namespace: corev1.NamespaceDefault,
		}

		storage = &nnfv1alpha1.NnfStorage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      storageKey.Name,
				Namespace: storageKey.Namespace,
			},
		}

		access = &nnfv1alpha1.NnfAccess{
			ObjectMeta: metav1.ObjectMeta{
				Name:      storageKey.Name,
				Namespace: storageKey.Namespace,
			},
			Spec: nnfv1alpha1.NnfAccessSpec{
				Target:        "all",
				DesiredState:  "mounted",
				TeardownState: "data_out",
				StorageReference: corev1.ObjectReference{
					Kind:      reflect.TypeOf(nnfv1alpha1.NnfStorage{}).Name(),
					Name:      storage.Name,
					Namespace: storage.Namespace,
				},
			},
		}
	})

	// After each test delete the NNF Storage, NNF Access and the Node
	AfterEach(func() {
		storage := &nnfv1alpha1.NnfStorage{}
		Expect(k8sClient.Get(context.TODO(), storageKey, storage)).To(Succeed())
		Expect(k8sClient.Delete(context.TODO(), storage)).To(Succeed())

		access := &nnfv1alpha1.NnfAccess{}
		Expect(k8sClient.Get(context.TODO(), storageKey, access)).To(Succeed())
		Expect(k8sClient.Delete(context.TODO(), access)).To(Succeed())
	})

	// Just before each test, ensure the NNF Storage resource is created. This is
	// outside the BeforeEach() declartion so each test can modify the NNF Storage
	// resource as needed. Same for NNF Access
	JustBeforeEach(func() {
		By("Create the NnfStorage and NnfAccess")
		mgsNode := storage.Status.MgsNode
		Expect(k8sClient.Create(context.TODO(), storage)).To(Succeed())
		Eventually(func() error {
			expected := &nnfv1alpha1.NnfStorage{}
			return k8sClient.Get(context.TODO(), storageKey, expected)
		}).Should(Succeed(), "create the nnf storage resource")

		if len(mgsNode) > 0 {
			storage.Status.MgsNode = mgsNode
			Expect(k8sClient.Status().Update(context.TODO(), storage)).To(Succeed())
			Eventually(func(g Gomega) string {
				expected := &nnfv1alpha1.NnfStorage{}
				g.Expect(k8sClient.Get(context.TODO(), storageKey, expected)).To(Succeed())
				return expected.Status.MgsNode
			}).Should(Equal(mgsNode), "update the nnf storage resource status")
		}
		Expect(k8sClient.Get(context.TODO(), storageKey, storage)).To(Succeed())

		Expect(k8sClient.Create(context.TODO(), access)).To(Succeed())
		Eventually(func() error {
			expected := &nnfv1alpha1.NnfAccess{}
			return k8sClient.Get(context.TODO(), storageKey, expected)
		}).Should(Succeed(), "create the nnf access resource")
	})

	// Before each test, create a skeletal template for the Data Movement resource.
	BeforeEach(func() {
		By("Define the NnfDataMovement")
		dmKey = types.NamespacedName{
			Name:      "dm-test-" + uuid.NewString()[0:6],
			Namespace: corev1.NamespaceDefault,
		}

		dm = &nnfv1alpha1.NnfDataMovement{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dmKey.Name,
				Namespace: dmKey.Namespace,
			},
			Spec: nnfv1alpha1.NnfDataMovementSpec{
				Source: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
					Access: &corev1.ObjectReference{
						Kind:      reflect.TypeOf(nnfv1alpha1.NnfAccess{}).Name(),
						Name:      access.Name,
						Namespace: access.Namespace,
					},
				},
				Destination: &nnfv1alpha1.NnfDataMovementSpecSourceDestination{
					Access: &corev1.ObjectReference{
						Kind:      reflect.TypeOf(nnfv1alpha1.NnfAccess{}).Name(),
						Name:      access.Name,
						Namespace: access.Namespace,
					},
				},
				UserId:  uint32(os.Getuid()),
				GroupId: uint32(os.Getgid()),
			},
		}
	})

	// Just before each test, ensure the Data Movement resource is created. This is
	// outside the BeforeEach() declartion so each test can modify the Data Movement
	// resource as needed.
	JustBeforeEach(func() {
		By("Create the NnfDataMovement")
		Expect(k8sClient.Create(context.TODO(), dm)).To(Succeed())
		Eventually(func() error {
			return k8sClient.Get(context.TODO(), dmKey, dm)
		}).Should(Succeed(), "create the data movement resource")
	})

	createGlobalLustre := func() {
		mgsNids := globalMGS

		lustre = &lusv1alpha1.LustreFileSystem{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "lustre-test",
				Namespace: corev1.NamespaceDefault,
			},
			Spec: lusv1alpha1.LustreFileSystemSpec{
				Name:      "lustre",
				MgsNids:   mgsNids,
				MountRoot: "/lus/test",
			},
		}
		Expect(k8sClient.Create(context.TODO(), lustre)).To(Succeed())
	}

	// Add source and dest info to NnfDataMovement.
	populateSourceAndDest := func() {
		dm.Spec.Source.Path = "example.file"
		dm.Spec.Source.Storage = &corev1.ObjectReference{
			Kind:      reflect.TypeOf(lusv1alpha1.LustreFileSystem{}).Name(),
			Name:      lustre.Name,
			Namespace: lustre.Namespace,
		}

		dm.Spec.Destination.Path = "/"
		dm.Spec.Destination.Storage = &corev1.ObjectReference{
			Kind:      reflect.TypeOf(nnfv1alpha1.NnfStorage{}).Name(),
			Name:      storage.Name,
			Namespace: storage.Namespace,
		}
	}

	Describe("Perform various Lustre to Lustre tests", func() {

		verifyNodeLabels := func() {
			for _, nodeKey := range nodeKeys {
				Eventually(func(g Gomega) map[string]string {
					node := &corev1.Node{}
					g.Expect(k8sClient.Get(context.TODO(), nodeKey, node)).To(Succeed())
					return node.Labels
				}).Should(HaveKeyWithValue(dmKey.Name, "true"))
			}
		}

		verifyPVandPVC := func(expectedMGS string) {
			pv := &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dmKey.Name + persistentVolumeSuffix,
					Namespace: dmKey.Namespace,
				},
			}

			Eventually(func() error {
				return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(pv), pv)
			}).Should(Succeed())

			Expect(*pv.Spec.CSI).To(MatchFields(IgnoreExtras, Fields{
				"Driver":       Equal("lustre-csi.hpe.com"),
				"FSType":       Equal("lustre"),
				"VolumeHandle": Equal(expectedMGS + ":/" + fsName),
			}))

			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dmKey.Name + persistentVolumeClaimSuffix,
					Namespace: "nnf-dm-system",
				},
			}

			Eventually(func() error {
				return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(pvc), pvc)
			}).Should(Succeed())

			Expect(pvc.Spec.VolumeName).To(Equal(pv.GetName()))
		}

		verifyMPIJob := func() {
			mpi := &mpiv2beta1.MPIJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dmKey.Name + mpiJobSuffix,
					Namespace: "nnf-dm-system",
				},
			}

			Eventually(func() error {
				return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(mpi), mpi)
			}).Should(Succeed(), "retrieve the mpi job")

			By("Checking the worker specification")
			worker := mpi.Spec.MPIReplicaSpecs[mpiv2beta1.MPIReplicaTypeWorker]
			Expect(*(worker.Replicas)).To(Equal(int32(testNumberOfNodes)))

			workerSpec := worker.Template.Spec
			source := corev1.PersistentVolumeClaimVolumeSource{ClaimName: lustre.Name + "-pvc"}
			destination := corev1.PersistentVolumeClaimVolumeSource{ClaimName: dm.Name + "-pvc"}

			Expect(workerSpec.Volumes).To(ContainElements(
				HaveField("VolumeSource.PersistentVolumeClaim", PointTo(Equal(source))),
				HaveField("VolumeSource.PersistentVolumeClaim", PointTo(Equal(destination))),
				//HaveField("VolumeSource.PersistentVolumeClaim.ClaimName", Equal(lustre.Name + "-pvc")), // NJR: Not sure why this isn't working, seems it can't dereference a pointer type
			), "have correct pvcs")
		}

		// These are the nodes that should be targeted for the allocations.
		getNodesForAllocationSets := func() []nnfv1alpha1.NnfStorageAllocationNodes {
			nodes := make([]nnfv1alpha1.NnfStorageAllocationNodes, len(nodeKeys))
			for nodeKeyIdx, nodeKey := range nodeKeys {
				nodes[nodeKeyIdx] = nnfv1alpha1.NnfStorageAllocationNodes{
					Name:  nodeKey.Name,
					Count: 1,
				}
			}
			return nodes
		}

		allocationSetsWithMGTMDT := func() []nnfv1alpha1.NnfStorageAllocationSetSpec {
			return []nnfv1alpha1.NnfStorageAllocationSetSpec{
				// Non OST definitions should be ignored
				{
					Name: "test-nnf-storage-mdt",
					NnfStorageLustreSpec: nnfv1alpha1.NnfStorageLustreSpec{
						TargetType:     "MGTMDT",
						FileSystemName: fsName,
					},
					Nodes: []nnfv1alpha1.NnfStorageAllocationNodes{},
				},
				{
					Name:     "test-nnf-storage",
					Capacity: 0,
					NnfStorageLustreSpec: nnfv1alpha1.NnfStorageLustreSpec{
						TargetType:     "OST",
						FileSystemName: fsName,
					},
					Nodes: getNodesForAllocationSets(),
				},
			}
		}

		allocationSetsWithExternalMGT := func() []nnfv1alpha1.NnfStorageAllocationSetSpec {
			return []nnfv1alpha1.NnfStorageAllocationSetSpec{
				// Non OST definitions should be ignored
				{
					Name: "test-nnf-storage-mdt",
					NnfStorageLustreSpec: nnfv1alpha1.NnfStorageLustreSpec{
						TargetType:     "MDT",
						FileSystemName: fsName,
						ExternalMgsNid: externalMGS,
					},
					Nodes: []nnfv1alpha1.NnfStorageAllocationNodes{},
				},
				{
					Name:     "test-nnf-storage",
					Capacity: 0,
					NnfStorageLustreSpec: nnfv1alpha1.NnfStorageLustreSpec{
						TargetType:     "OST",
						FileSystemName: fsName,
						ExternalMgsNid: externalMGS,
					},
					Nodes: getNodesForAllocationSets(),
				},
			}
		}

		// Add AllocationSets to NnfStorage.
		populateAllocationSets := func(getAllocationSets func() []nnfv1alpha1.NnfStorageAllocationSetSpec) {
			// getAllocationSets is a func ptr, because we need to set 'fsname' for it.
			fsName = "lustre1"
			storage.Spec = nnfv1alpha1.NnfStorageSpec{
				FileSystemType: "lustre",
				AllocationSets: getAllocationSets(),
			}
		}

		Context("When source is Lustre File System type", func() {

			BeforeEach(func() {
				By("Create the global lustre filesystem")
				createGlobalLustre()
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(context.TODO(), lustre)).To(Succeed())
			})

			Context("When destination is Nnf Storage type", func() {

				BeforeEach(func() {
					By("Add AllocationSets to NnfStorage")
					populateAllocationSets(allocationSetsWithMGTMDT)
				})

				BeforeEach(func() {
					By("Define MgsNode in NnfStorage")

					storage.Status.MgsNode = orchestratedMGS
				})

				BeforeEach(func() {
					By("Add source and dest info to NnfDataMovement")
					populateSourceAndDest()
				})

				JustBeforeEach(func() {
					By("Validate Condition array of NnfDataMovement")
					Eventually(func(g Gomega) []metav1.Condition {
						expected := &nnfv1alpha1.NnfDataMovement{}
						g.Expect(k8sClient.Get(context.TODO(), dmKey, expected)).To(Succeed())
						return expected.Status.Conditions
					}).Should(ContainElements(
						HaveField("Type", nnfv1alpha1.DataMovementConditionTypeStarting),
						HaveField("Type", nnfv1alpha1.DataMovementConditionTypeRunning),
					), "transition to running")
				})

				Describe("Create Data Movement resource", func() {
					// After each life-cycle test specification, delete the Data Movement resource
					AfterEach(func() {
						expected := &nnfv1alpha1.NnfDataMovement{}
						Expect(k8sClient.Get(context.TODO(), dmKey, expected)).To(Succeed())
						Expect(k8sClient.Delete(context.TODO(), expected)).To(Succeed())

						Eventually(func() error {
							return k8sClient.Get(context.TODO(), dmKey, expected)
						}).ShouldNot(Succeed())
					})

					It("Labels the node", func() {
						verifyNodeLabels()
					})

					It("Creates PV/PVC", func() {
						verifyPVandPVC(orchestratedMGS)
					})

					It("Creates MPIJob", func() {
						verifyMPIJob()
					})
				}) // Describe("Create Data Movement resource")

				Describe("Create Data Movement resource with configuration map", func() {

					AfterEach(func() {
						expected := &nnfv1alpha1.NnfDataMovement{}
						Expect(k8sClient.Get(context.TODO(), dmKey, expected)).To(Succeed())
						Expect(k8sClient.Delete(context.TODO(), expected)).To(Succeed())
					})

					// Create the ConfigMap this block will refer to
					var (
						config *corev1.ConfigMap
					)

					BeforeEach(func() {
						config = &corev1.ConfigMap{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "data-movement" + configSuffix,
								Namespace: configNamespace,
							},
							Data: map[string]string{
								configImage:             "testImage",
								configCommand:           "testCommand",
								configSourceVolume:      `{ "hostPath": { "path": "/tmp", "type": "Directory" } }`,
								configDestinationVolume: `{ "hostPath": { "path": "/tmp", "type": "Directory" } }`,
							},
						}

						Expect(k8sClient.Create(context.TODO(), config)).To(Succeed())
					})

					AfterEach(func() {
						Expect(k8sClient.Delete(context.TODO(), config)).To(Succeed())
					})

					It("Contains correct overrides", func() {

						mpi := &mpiv2beta1.MPIJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      dmKey.Name + mpiJobSuffix,
								Namespace: "nnf-dm-system",
							},
						}

						Eventually(func() error {
							return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(mpi), mpi)
						}).Should(Succeed())

						By("Checking the launcher specification")
						launcherSpec := mpi.Spec.MPIReplicaSpecs[mpiv2beta1.MPIReplicaTypeLauncher].Template.Spec
						Expect(launcherSpec.Containers).To(HaveLen(1))

						By("Checking the worker specification")
						workerSpec := mpi.Spec.MPIReplicaSpecs[mpiv2beta1.MPIReplicaTypeWorker].Template.Spec
						Expect(workerSpec.Containers).To(HaveLen(1))
						Expect(workerSpec.Containers[0].Image == config.Data[configImage])

						hostPathType := corev1.HostPathDirectory
						source := corev1.HostPathVolumeSource{Path: "/tmp", Type: &hostPathType}
						destination := corev1.HostPathVolumeSource{Path: "/tmp", Type: &hostPathType}

						Expect(workerSpec.Volumes).To(ContainElements(
							HaveField("VolumeSource.HostPath", PointTo(Equal(source))),
							HaveField("VolumeSource.HostPath", PointTo(Equal(destination))),
						))

					})
				})

				Describe("Delete Data Movement resource", func() {

					JustBeforeEach(func() {
						expected := &nnfv1alpha1.NnfDataMovement{}
						Expect(k8sClient.Get(context.TODO(), dmKey, expected)).To(Succeed())
						Expect(k8sClient.Delete(context.TODO(), dm)).To(Succeed())
					})

					It("Unlabels the nodes", func() {
						for _, nodeKey := range nodeKeys {
							Eventually(func(g Gomega) map[string]string {
								node := &corev1.Node{}
								g.Expect(k8sClient.Get(context.TODO(), nodeKey, node)).To(Succeed())
								return node.Labels
							}).ShouldNot(HaveKey(dmKey.Name))
						}
					})
				})
			}) // Context("When destination is NnfStorage Instance type")

			Context("When destination is NnfStorage type using external MGS", func() {

				BeforeEach(func() {
					By("Add AllocationSets to NnfStorage")
					populateAllocationSets(allocationSetsWithExternalMGT)
				})

				BeforeEach(func() {
					By("Add source and dest info to NnfDataMovement")
					populateSourceAndDest()
				})

				JustBeforeEach(func() {
					By("Validate Condition array of NnfDataMovement")
					Eventually(func(g Gomega) []metav1.Condition {
						expected := &nnfv1alpha1.NnfDataMovement{}
						g.Expect(k8sClient.Get(context.TODO(), dmKey, expected)).To(Succeed())
						return expected.Status.Conditions
					}).Should(ContainElements(
						HaveField("Type", nnfv1alpha1.DataMovementConditionTypeStarting),
						HaveField("Type", nnfv1alpha1.DataMovementConditionTypeRunning),
					), "transition to running")
				})

				Describe("Create Data Movement resource using external MGS", func() {
					// After each life-cycle test specification, delete the Data Movement resource
					AfterEach(func() {
						expected := &nnfv1alpha1.NnfDataMovement{}
						Expect(k8sClient.Get(context.TODO(), dmKey, expected)).To(Succeed())
						Expect(k8sClient.Delete(context.TODO(), expected)).To(Succeed())

						Eventually(func() error {
							return k8sClient.Get(context.TODO(), dmKey, expected)
						}).ShouldNot(Succeed())
					})

					It("Labels the node", func() {
						verifyNodeLabels()
					})

					It("Creates PV/PVC", func() {
						verifyPVandPVC(externalMGS)
					})

					It("Creates MPIJob", func() {
						verifyMPIJob()
					})
				}) // Describe("Create Data Movement resource using external MGS")
			}) // Context("When destination is NnfStorage type using external MGS")

			Context("When destination is Persistent File System of lustre type", func() {})

		}) // Context("When source is Lustre File System type")

		Context("When source is Persistent File System of lustre type", func() {})

	}) // Describe("Perform various Lustre to Lustre tests")

	Describe("Perform various Lustre to XFS/GFS2 tests", func() {

		BeforeEach(func() {
			storage.Spec = nnfv1alpha1.NnfStorageSpec{
				FileSystemType: "xfs",
				AllocationSets: []nnfv1alpha1.NnfStorageAllocationSetSpec{
					{
						Name:     "test-nnf-storage-xfs",
						Capacity: 0,
						Nodes: []nnfv1alpha1.NnfStorageAllocationNodes{
							{
								Name:  nodeKeys[0].Name,
								Count: 1,
							},
						},
					},
				},
			}
		})

		// Create the ConfigMap this block will refer to
		var (
			config *corev1.ConfigMap
		)

		BeforeEach(func() {
			_, err := os.Create("test.in")
			Expect(err).NotTo(HaveOccurred())

			config = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "data-movement" + configSuffix,
					Namespace: configNamespace,
				},
				Data: map[string]string{
					configSourcePath:      "test.in",
					configDestinationPath: "test.out",
				},
			}

			Expect(k8sClient.Create(context.TODO(), config)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.TODO(), config)).To(Succeed())

			Eventually(func() error {
				return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(config), config)
			}).WithTimeout(10 * time.Second).ShouldNot(Succeed())
		})

		Context("When source is Lustre File System type", func() {

			BeforeEach(func() {
				By("Create the global lustre filesystem")
				createGlobalLustre()
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(context.TODO(), lustre)).To(Succeed())
			})

			Context("When destination is Job Storage Instance type", func() {

				BeforeEach(func() {
					// Exact paths don't matter because we're using overrides.
					populateSourceAndDest()
				})

				// We expect data movement to enter at least a running state (possibly more)
				JustBeforeEach(func() {
					Eventually(func(g Gomega) []metav1.Condition {
						expected := &nnfv1alpha1.NnfDataMovement{}
						g.Expect(k8sClient.Get(context.TODO(), dmKey, expected)).To(Succeed())
						return expected.Status.Conditions
					}, "3s").Should(ContainElements(
						HaveField("Type", nnfv1alpha1.DataMovementConditionTypeStarting),
						HaveField("Type", nnfv1alpha1.DataMovementConditionTypeRunning),
					), "transition to running")
				})

				Describe("Rsync Data Movement", func() {

					It("Validates full rsync data movement lifecycle", func() {
						Expect(storage.Spec.AllocationSets).To(HaveLen(1), "Expected allocation set count incorrect - did you forget to change the test logic?")
						Expect(storage.Spec.AllocationSets[0].Nodes).To(HaveLen(1), "Expected node count incorrect - did you forget to change the test logic?")
						expectedRsyncNodeCount := storage.Spec.AllocationSets[0].Nodes[0].Count

						rsyncNodes := &dmv1alpha1.RsyncNodeDataMovementList{}

						Eventually(func(g Gomega) []dmv1alpha1.RsyncNodeDataMovement {
							g.Expect(k8sClient.List(context.TODO(), rsyncNodes, dwsv1alpha1.MatchingOwner(dm))).To(Succeed())
							return rsyncNodes.Items
						}).Should(HaveLen(expectedRsyncNodeCount), "expected number of rsync nodes")

						for _, item := range rsyncNodes.Items {
							Expect(item.ObjectMeta.Labels).To(HaveKeyWithValue(dwsv1alpha1.OwnerNameLabel, dm.Name))

							// TODO: Expect the correct Source and Destination paths. Source should be the lustre volume
						}

						// Validate Rsync Nodes finish with success
						for _, item := range rsyncNodes.Items {
							Eventually(func(g Gomega) dmv1alpha1.RsyncNodeDataMovementStatus {
								expected := &dmv1alpha1.RsyncNodeDataMovement{}
								g.Expect(k8sClient.Get(context.TODO(), types.NamespacedName{Name: item.Name, Namespace: item.Namespace}, expected)).To(Succeed())
								return expected.Status
							}).Should(MatchFields(IgnoreExtras, Fields{
								"State":   Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
								"Status":  Equal(nnfv1alpha1.DataMovementConditionReasonSuccess),
								"Message": BeEmpty(),
							}))
						}

						// Validate the Data Movement finishes wtih success
						Eventually(func(g Gomega) []metav1.Condition {
							g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
							return dm.Status.Conditions
						}).Should(ContainElements(MatchFields(IgnoreExtras, Fields{
							"Type":   Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
							"Reason": Equal(nnfv1alpha1.DataMovementConditionReasonSuccess),
							"Status": Equal(metav1.ConditionTrue),
						})))

						// Delete the data movement resource
						Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
						Expect(k8sClient.Delete(context.TODO(), dm)).To(Succeed())

						Eventually(func() error {
							return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)
						}).ShouldNot(Succeed(), "Expect data movement to delete fully")

						// Validate the Rsync Nodes delete
						Eventually(func(g Gomega) []dmv1alpha1.RsyncNodeDataMovement {
							g.Expect(k8sClient.List(context.TODO(), rsyncNodes, dwsv1alpha1.MatchingOwner(dm))).To(Succeed())
							return rsyncNodes.Items
						}).Should(BeEmpty(), "expected zero rsync nodes on delete")
					})
				})
			})

			Context("When data movement configured to monitor only", func() {

				var rsync *dmv1alpha1.RsyncNodeDataMovement = nil

				BeforeEach(func() {
					// Clear the source & destination spec; data movement will enter monitor mode
					// where rsync jobs are strictly observed.
					dm.Spec.Source = nil
					dm.Spec.Destination = nil

					dm.Spec.Monitor = true
				})

				JustBeforeEach(func() {
					Eventually(func(g Gomega) []metav1.Condition {
						expected := &nnfv1alpha1.NnfDataMovement{}
						g.Expect(k8sClient.Get(context.TODO(), dmKey, expected)).To(Succeed())
						return expected.Status.Conditions
					}, "3s").Should(ContainElements(
						HaveField("Type", nnfv1alpha1.DataMovementConditionTypeStarting),
						HaveField("Type", nnfv1alpha1.DataMovementConditionTypeRunning),
					), "transition to running")
				})

				BeforeEach(func() {
					rsync = &dmv1alpha1.RsyncNodeDataMovement{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-rsync-job",
							Namespace: nodeKeys[0].Name,
						},
						Spec: dmv1alpha1.RsyncNodeDataMovementSpec{
							Source:      "test.in",
							Destination: "test.out",
							UserId:      uint32(os.Getuid()),
							GroupId:     uint32(os.Getgid()),
						},
					}
					dwsv1alpha1.AddOwnerLabels(rsync, dm)
				})

				JustBeforeEach(func() {
					By("Creating an rsync job")
					Expect(k8sClient.Create(context.TODO(), rsync)).To(Succeed())

					Eventually(func() error {
						return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(rsync), rsync)
					}).Should(Succeed())

					Eventually(func(g Gomega) string {
						g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(rsync), rsync)).To(Succeed())
						return rsync.Status.State
					}).Should(Equal(nnfv1alpha1.DataMovementConditionTypeFinished))
				})

				AfterEach(func() {
					// Deleting the data movement resource should also clean up the rsync jobs
					Expect(k8sClient.Delete(context.TODO(), dm)).To(Succeed())

					Eventually(func() error {
						return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)
					}).WithTimeout(10 * time.Second).ShouldNot(Succeed())

					rsyncs := &dmv1alpha1.RsyncNodeDataMovementList{}
					Expect(k8sClient.List(context.TODO(), rsyncs)).To(Succeed())
					Expect(rsyncs.Items).To(BeEmpty())
				})

				stopDataMovementMonitoring := func() {
					Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
					Expect(dm.Spec.Monitor).To(BeTrue())
					dm.Spec.Monitor = false
					Expect(k8sClient.Update(context.TODO(), dm)).To(Succeed())
				}

				checkDataMovementFinishesWithReason := func(reason string) {

					Eventually(func(g Gomega) metav1.Condition {
						g.Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
						return dm.Status.Conditions[len(dm.Status.Conditions)-1]
					}).WithTimeout(time.Second).Should(MatchFields(IgnoreExtras, Fields{
						"Type":   Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
						"Reason": Equal(reason),
					}))
				}

				Describe("Monitors successful rsync job", func() {

					It("successfully reports rsync status", func() {
						Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(rsync), rsync)).To(Succeed())
						Expect(rsync.Status.Status).To(Equal(nnfv1alpha1.DataMovementConditionReasonSuccess))

						By("Stop data movement monitoring")
						stopDataMovementMonitoring()

						By("Checking that data movement is successful")
						checkDataMovementFinishesWithReason(nnfv1alpha1.DataMovementConditionReasonSuccess)
					})

				})

				Describe("Monitors unsuccessful rsync job", func() {

					BeforeEach(func() {
						rsync.Spec.Source = "invalid.file"
					})

					It("successfully reports failed rysnc status", func() {
						Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(rsync), rsync)).To(Succeed())
						Expect(rsync.Status.Status).To(Equal(nnfv1alpha1.DataMovementConditionReasonFailed))

						By("Stop data movement monitoring")
						stopDataMovementMonitoring()

						By("Checking that data movement is unsuccesful")
						checkDataMovementFinishesWithReason(nnfv1alpha1.DataMovementConditionReasonFailed)
					})

				})

			})
		})

	})
})

package controllers

import (
	"context"
	"os"
	"reflect"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	mpiv2beta1 "github.com/kubeflow/mpi-operator/v2/pkg/apis/kubeflow/v2beta1"

	lusv1alpha1 "github.hpe.com/hpe/hpc-rabsw-lustre-fs-operator/api/v1alpha1"
	dmv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-sos/api/v1alpha1"
)

const (
	testNumberOfNodes = 2
)

var _ = Describe("Data Movement Controller", func() {

	var (
		nodeKeys          []types.NamespacedName
		storageKey, dmKey types.NamespacedName
		storage           *nnfv1alpha1.NnfStorage
		access            *nnfv1alpha1.NnfAccess
		dm                *nnfv1alpha1.NnfDataMovement
		dmns              *corev1.Namespace
		dmOwnerRef        metav1.OwnerReference
	)

	// Create the nnf-dm-system namespace that is used by data-movement
	BeforeEach(func() {
		dmns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: configNamespace,
		}}

		Expect(k8sClient.Create(context.TODO(), dmns)).Should(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(context.TODO(), dmns)).Should(Succeed())
	})

	// Before each test ensure there is a Node with the proper label (cray.nnf.node=true), and
	// there is NNF Storage that contains that node as its one and only allocation.
	BeforeEach(func() {
		nodeKeys = []types.NamespacedName{
			{Name: "test-node-0"},
			{Name: "test-node-1"},
		}

		for _, nodeKey := range nodeKeys {
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

		for _, nodeKey := range nodeKeys {
			node := &corev1.Node{}
			Expect(k8sClient.Get(context.TODO(), nodeKey, node)).To(Succeed())
			Expect(k8sClient.Delete(context.TODO(), node)).To(Succeed())
		}
	})

	// Just before each test, ensure the NNF Storage resource is created. This is
	// outside the BeforeEach() declartion so each test can modify the NNF Storage
	// resource as needed. Same for NNF Access
	JustBeforeEach(func() {
		mgsNode := storage.Status.MgsNode
		Expect(k8sClient.Create(context.TODO(), storage)).To(Succeed())
		Eventually(func() error {
			expected := &nnfv1alpha1.NnfStorage{}
			return k8sClient.Get(context.TODO(), storageKey, expected)
		}).Should(Succeed(), "create the nnf storage resource")

		if len(mgsNode) > 0 {
			storage.Status.MgsNode = mgsNode
			Expect(k8sClient.Status().Update(context.TODO(), storage)).To(Succeed())
			Eventually(func() string {
				expected := &nnfv1alpha1.NnfStorage{}
				Expect(k8sClient.Get(context.TODO(), storageKey, expected)).To(Succeed())
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
		dmKey = types.NamespacedName{
			Name:      "nnf-data-mover-job-123", // TODO: Randomize the name so we don't get conflicts during test execution
			Namespace: corev1.NamespaceDefault,
		}

		dm = &nnfv1alpha1.NnfDataMovement{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dmKey.Name,
				Namespace: dmKey.Namespace,
			},
			Spec: nnfv1alpha1.NnfDataMovementSpec{
				Source: nnfv1alpha1.NnfDataMovementSpecSourceDestination{
					Access: &corev1.ObjectReference{
						Kind:      reflect.TypeOf(nnfv1alpha1.NnfAccess{}).Name(),
						Name:      access.Name,
						Namespace: access.Namespace,
					},
				},
				Destination: nnfv1alpha1.NnfDataMovementSpecSourceDestination{
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
		Expect(k8sClient.Create(context.TODO(), dm)).To(Succeed())
		Eventually(func() error {
			return k8sClient.Get(context.TODO(), dmKey, dm)
		}).Should(Succeed(), "create the data movement resource")

		controller := true
		blockOwnerDeletion := true
		dmOwnerRef = metav1.OwnerReference{
			Kind:               "NnfDataMovement",
			APIVersion:         nnfv1alpha1.GroupVersion.String(),
			UID:                dm.GetUID(),
			Name:               dm.Name,
			Controller:         &controller,
			BlockOwnerDeletion: &blockOwnerDeletion,
		}
	})

	Describe("Perform various Lustre to Lustre tests", func() {

		Context("When source is Lustre File System type", func() {
			var (
				lustre *lusv1alpha1.LustreFileSystem
			)

			BeforeEach(func() {
				// These are the nodes that should be targeted for the
				nodes := make([]nnfv1alpha1.NnfStorageAllocationNodes, len(nodeKeys))
				for nodeKeyIdx, nodeKey := range nodeKeys {
					nodes[nodeKeyIdx] = nnfv1alpha1.NnfStorageAllocationNodes{
						Name:  nodeKey.Name,
						Count: 1,
					}
				}

				storage.Spec = nnfv1alpha1.NnfStorageSpec{
					FileSystemType: "lustre",
					AllocationSets: []nnfv1alpha1.NnfStorageAllocationSetSpec{
						// Non OST definitions should be ignored
						{
							Name: "test-nnf-storage-mdt",
							NnfStorageLustreSpec: nnfv1alpha1.NnfStorageLustreSpec{
								TargetType: "MDT",
							},
							Nodes: []nnfv1alpha1.NnfStorageAllocationNodes{},
						},
						{
							Name:     "test-nnf-storage",
							Capacity: 0,
							NnfStorageLustreSpec: nnfv1alpha1.NnfStorageLustreSpec{
								TargetType: "OST",
							},
							Nodes: nodes,
						},
					},
				}
			})

			BeforeEach(func() {
				lustre = &lusv1alpha1.LustreFileSystem{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "lustre-test",
						Namespace: corev1.NamespaceDefault,
					},
					Spec: lusv1alpha1.LustreFileSystemSpec{
						Name:      "lustre",
						MgsNid:    "172.0.0.1@tcp",
						MountRoot: "/lus/test",
					},
				}
				Expect(k8sClient.Create(context.TODO(), lustre)).To(Succeed())
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(context.TODO(), lustre)).To(Succeed())
			})

			Context("When destination is Nnf Storage type", func() {

				BeforeEach(func() {

					storage.Status.MgsNode = "172.0.0.1@tcp"

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

				})

				JustBeforeEach(func() {
					Eventually(func() []metav1.Condition {
						expected := &nnfv1alpha1.NnfDataMovement{}
						Expect(k8sClient.Get(context.TODO(), dmKey, expected)).To(Succeed())
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
					})

					PIt("Labels the node", func() {
						for _, nodeKey := range nodeKeys {
							Eventually(func() map[string]string {
								node := &corev1.Node{}
								Expect(k8sClient.Get(context.TODO(), nodeKey, node)).To(Succeed())
								return node.Labels
							}).Should(HaveKeyWithValue(dmKey.Name, "true"))
						}
					})

					PIt("Creates PV/PVC", func() {
						pv := &corev1.PersistentVolume{
							ObjectMeta: metav1.ObjectMeta{
								Name:      dmKey.Name + persistentVolumeSuffix,
								Namespace: dmKey.Namespace,
							},
						}

						Eventually(func() error {
							return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(pv), pv)
						}).Should(Succeed())

						Expect(pv.ObjectMeta.OwnerReferences).To(ContainElement(dmOwnerRef))

						Expect(*pv.Spec.CSI).To(MatchFields(IgnoreExtras, Fields{
							"Driver":       Equal("lustre-csi.nnf.cray.hpe.com"),
							"FSType":       Equal("lustre"),
							"VolumeHandle": Equal(storage.Status.MgsNode),
						}))

						pvc := &corev1.PersistentVolumeClaim{
							ObjectMeta: metav1.ObjectMeta{
								Name:      dmKey.Name + persistentVolumeClaimSuffix,
								Namespace: dmKey.Namespace,
							},
						}

						Eventually(func() error {
							return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(pvc), pvc)
						}).Should(Succeed())

						Expect(pvc.ObjectMeta.OwnerReferences).To(ContainElement(dmOwnerRef))

						Expect(pvc.Spec.VolumeName).To(Equal(pv.GetName()))
					})

					PIt("Creates MPIJob", func() {

						mpi := &mpiv2beta1.MPIJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      dmKey.Name + mpiJobSuffix,
								Namespace: dmKey.Namespace,
							},
						}

						Eventually(func() error {
							return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(mpi), mpi)
						}).Should(Succeed(), "retrieve the mpi job")

						Expect(mpi.ObjectMeta.OwnerReferences).To(ContainElement(dmOwnerRef))

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

					PIt("Contains correct overrides", func() {

						mpi := &mpiv2beta1.MPIJob{
							ObjectMeta: metav1.ObjectMeta{
								Name:      dmKey.Name + mpiJobSuffix,
								Namespace: dmKey.Namespace,
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

					PIt("Unlabels the nodes", func() {
						for _, nodeKey := range nodeKeys {
							Eventually(func() map[string]string {
								node := &corev1.Node{}
								Expect(k8sClient.Get(context.TODO(), nodeKey, node)).To(Succeed())
								return node.Labels
							}).ShouldNot(HaveKey(dmKey.Name))
						}
					})
				})
			}) // Context("When destination is Job Storage Instance type")

			Context("When destination is Persistent File System of lustre type", func() {})

		}) // Context("When source is Lustre File System type")

		Context("When source is Persistent File System of lustre type", func() {})

	}) // Describe("Perform various Lustre to Lustre tests")

	Describe("Perform various Lustre to XFS/GFS2 tests", func() {
		var (
			ns *corev1.Namespace
		)

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

		// Create a namespace for receiving the Rsync Node's data movement resource
		BeforeEach(func() {
			ns = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeKeys[0].Name,
				},
			}

			Expect(k8sClient.Create(context.TODO(), ns)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.TODO(), ns)).To(Succeed())
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
		})

		Context("When source is Lustre File System type", func() {
			var (
				lustre *lusv1alpha1.LustreFileSystem
			)

			BeforeEach(func() {
				lustre = &lusv1alpha1.LustreFileSystem{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "lustre-test",
						Namespace: corev1.NamespaceDefault,
					},
					Spec: lusv1alpha1.LustreFileSystemSpec{
						Name:      "lustre",
						MgsNid:    "172.0.0.1@tcp",
						MountRoot: "/lus/test",
					},
				}
				Expect(k8sClient.Create(context.TODO(), lustre)).To(Succeed())
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(context.TODO(), lustre)).To(Succeed())
			})

			Context("When destination is Job Storage Instance type", func() {

				BeforeEach(func() {
					dm.Spec.Source.Path = "/" // Doesn't matter, using overrides
					dm.Spec.Source.Storage = &corev1.ObjectReference{
						Kind:      reflect.TypeOf(lusv1alpha1.LustreFileSystem{}).Name(),
						Name:      lustre.Name,
						Namespace: lustre.Namespace,
					}

					dm.Spec.Destination.Path = "/" // Doesn't matter, using overrides
					dm.Spec.Destination.Storage = &corev1.ObjectReference{
						Kind:      reflect.TypeOf(nnfv1alpha1.NnfStorage{}).Name(),
						Name:      storage.Name,
						Namespace: storage.Namespace,
					}
				})

				JustBeforeEach(func() {
					Eventually(func() []metav1.Condition {
						expected := &nnfv1alpha1.NnfDataMovement{}
						Expect(k8sClient.Get(context.TODO(), dmKey, expected)).To(Succeed())
						return expected.Status.Conditions
					}, "3s").Should(ContainElements(
						HaveField("Type", nnfv1alpha1.DataMovementConditionTypeStarting),
						HaveField("Type", nnfv1alpha1.DataMovementConditionTypeRunning),
					), "transition to running")
				})

				Describe("Rsync Data Movement", func() {

					// Test disabled until rsync can be added to the docker build image
					PIt("Validates full rsync data movement lifecycle", func() {
						Expect(storage.Spec.AllocationSets).To(HaveLen(1), "Expected allocation set count incorrect - did you forget to change the test logic?")
						Expect(storage.Spec.AllocationSets[0].Nodes).To(HaveLen(1), "Expected node count incorrect - did you forget to change the test logic?")
						expectedRsyncNodeCount := storage.Spec.AllocationSets[0].Nodes[0].Count

						rsyncNodes := &dmv1alpha1.RsyncNodeDataMovementList{}

						Eventually(func() []dmv1alpha1.RsyncNodeDataMovement {
							Expect(k8sClient.List(context.TODO(), rsyncNodes, client.HasLabels{dmv1alpha1.OwnerLabelRsyncNodeDataMovement})).To(Succeed())
							return rsyncNodes.Items
						}).Should(HaveLen(expectedRsyncNodeCount), "expected number of rsync nodes")

						for _, item := range rsyncNodes.Items {
							Expect(item.ObjectMeta.Labels).To(HaveKeyWithValue(dmv1alpha1.OwnerLabelRsyncNodeDataMovement, dm.Name))
							Expect(item.ObjectMeta.Annotations).To(HaveKeyWithValue(dmv1alpha1.OwnerLabelRsyncNodeDataMovement, dm.Name+"/"+dm.Namespace))

							// TODO: Expect the correct Source and Destination paths. Source should be the lustre volume
						}

						// Validate Rsync Nodes finish with success
						for _, item := range rsyncNodes.Items {
							Eventually(func() dmv1alpha1.RsyncNodeDataMovementStatus {
								expected := &dmv1alpha1.RsyncNodeDataMovement{}
								Expect(k8sClient.Get(context.TODO(), types.NamespacedName{Name: item.Name, Namespace: item.Namespace}, expected)).To(Succeed())
								return expected.Status
							}).Should(MatchFields(IgnoreExtras, Fields{
								"State":   Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
								"Status":  Equal(nnfv1alpha1.DataMovementConditionReasonSuccess),
								"Message": BeEmpty(),
							}))
						}

						// Validate the Data Movement finishes wtih success
						Eventually(func() []metav1.Condition {
							Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm)).To(Succeed())
							return dm.Status.Conditions
						}).Should(ContainElements(MatchFields(IgnoreExtras, Fields{
							"Type":   Equal(nnfv1alpha1.DataMovementConditionTypeFinished),
							"Reason": Equal(nnfv1alpha1.DataMovementConditionReasonSuccess),
							"Status": Equal(metav1.ConditionTrue),
						})))

						// Delete the data movement resource
						expected := &nnfv1alpha1.NnfDataMovement{}
						Expect(k8sClient.Get(context.TODO(), dmKey, expected)).To(Succeed())
						Expect(k8sClient.Delete(context.TODO(), expected)).To(Succeed())

						// Validate the Rsync Nodes delete
						Eventually(func() []dmv1alpha1.RsyncNodeDataMovement {
							Expect(k8sClient.List(context.TODO(), rsyncNodes, client.HasLabels{dmv1alpha1.OwnerLabelRsyncNodeDataMovement})).To(Succeed())
							return rsyncNodes.Items
						}).Should(BeEmpty(), "expected zero rsync nodes on delete")
					})
				})
			})
		})

	})
})

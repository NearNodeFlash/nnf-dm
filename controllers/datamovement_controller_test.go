package controllers

import (
	"context"

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
		dm                *dmv1alpha1.DataMovement
	)

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
	})

	// After each test delete the NNF Storage and the Node
	AfterEach(func() {
		storage := &nnfv1alpha1.NnfStorage{}
		Expect(k8sClient.Get(context.TODO(), storageKey, storage)).To(Succeed())
		Expect(k8sClient.Delete(context.TODO(), storage)).To(Succeed())

		for _, nodeKey := range nodeKeys {
			node := &corev1.Node{}
			Expect(k8sClient.Get(context.TODO(), nodeKey, node)).To(Succeed())
			Expect(k8sClient.Delete(context.TODO(), node)).To(Succeed())
		}
	})

	// Just before each test, ensure the NNF Storage resource is created. This is
	// outside the BeforeEach() declartion so each test can modify the NNF Storage
	// resource as needed.
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
	})

	// Before each test, create a skeletal template for the Data Movement resource.
	BeforeEach(func() {
		dmKey = types.NamespacedName{
			Name:      "nnf-data-mover-job-123", // TODO: Randomize the name so we don't get conflicts during test execution
			Namespace: corev1.NamespaceDefault,
		}

		dm = &dmv1alpha1.DataMovement{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dmKey.Name,
				Namespace: dmKey.Namespace,
			},
			Spec: dmv1alpha1.DataMovementSpec{
				Storage: corev1.ObjectReference{
					Kind:      "NnfStorage",
					Name:      storage.Name,
					Namespace: storage.Namespace,
				},
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
					AllocationSets: []nnfv1alpha1.NnfStorageAllocationSetSpec{
						// Non OST definitions should be ignored
						{
							Name:           "test-nnf-storage-mdt",
							FileSystemType: "lustre",
							NnfStorageLustreSpec: nnfv1alpha1.NnfStorageLustreSpec{
								TargetType: "MDT",
							},
							Nodes: []nnfv1alpha1.NnfStorageAllocationNodes{},
						},
						{
							Name:           "test-nnf-storage",
							Capacity:       0,
							FileSystemType: "lustre",
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
						Name:      "lustre-test",
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
				var (
					jsi *nnfv1alpha1.NnfJobStorageInstance
				)

				BeforeEach(func() {
					jsi = &nnfv1alpha1.NnfJobStorageInstance{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "job-storage-instance-test",
							Namespace: corev1.NamespaceDefault,
						},
						Spec: nnfv1alpha1.NnfJobStorageInstanceSpec{
							Name:   "job-storage-instance-test",
							FsType: "lustre",
						},
					}

					Expect(k8sClient.Create(context.TODO(), jsi)).To(Succeed())
					Eventually(func() error {
						return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(jsi), jsi)
					}).Should(Succeed())

				})

				AfterEach(func() {
					Expect(k8sClient.Delete(context.TODO(), jsi)).To(Succeed())
				})

				BeforeEach(func() {

					storage.Status.MgsNode = "172.0.0.1@tcp"

					dm.Spec.Source = dmv1alpha1.DataMovementSpecSourceDestination{
						Path: "example.file",
						StorageInstance: &corev1.ObjectReference{
							Kind:      "LustreFileSystem",
							Name:      lustre.Name,
							Namespace: lustre.Namespace,
						},
					}

					dm.Spec.Destination = dmv1alpha1.DataMovementSpecSourceDestination{
						Path: "/",
						StorageInstance: &corev1.ObjectReference{
							Kind:      "NnfJobStorageInstance",
							Name:      jsi.Name,
							Namespace: jsi.Namespace,
						},
					}
				})

				JustBeforeEach(func() {
					Eventually(func() []metav1.Condition {
						expected := &dmv1alpha1.DataMovement{}
						Expect(k8sClient.Get(context.TODO(), dmKey, expected)).To(Succeed())
						return expected.Status.Conditions
					}).Should(ContainElements(
						HaveField("Type", dmv1alpha1.DataMovementConditionStarting),
						HaveField("Type", dmv1alpha1.DataMovementConditionRunning),
					), "transition to running")
				})

				Describe("Create Data Movement resource", func() {
					// After each life-cycle test specification, delete the Data Movement resource
					AfterEach(func() {
						expected := &dmv1alpha1.DataMovement{}
						Expect(k8sClient.Get(context.TODO(), dmKey, expected)).To(Succeed())
						Expect(k8sClient.Delete(context.TODO(), expected)).To(Succeed())
					})

					It("Labels the node", func() {
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

						Expect(pv.Spec.CSI).To(MatchAllFields(Fields{
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

						// TODO: Do some checking of the PVC attributes
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

						By("Checking the worker specification")
						workerSpec := mpi.Spec.MPIReplicaSpecs[mpiv2beta1.MPIReplicaTypeWorker].Template.Spec

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
						expected := &dmv1alpha1.DataMovement{}
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
								Namespace: corev1.NamespaceDefault,
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
						expected := &dmv1alpha1.DataMovement{}
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

})

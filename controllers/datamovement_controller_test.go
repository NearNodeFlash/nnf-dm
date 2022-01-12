package controllers

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	mpiv2beta1 "github.com/kubeflow/mpi-operator/v2/pkg/apis/kubeflow/v2beta1"

	dmv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-dm/api/v1alpha1"
	nnfv1alpha1 "github.hpe.com/hpe/hpc-rabsw-nnf-sos/api/v1alpha1"
	//lusv1alpha1 "github.hpe.com/hpe/hpc-rabsw-lustre-fs-operator/api/v1alpha1"
)

var _ = Describe("Data Movement Controller", func() {

	var (
		nodeKey, storageKey, dmKey types.NamespacedName
		storage                    *nnfv1alpha1.NnfStorage
		dm                         *dmv1alpha1.DataMovement
	)

	// Before each test ensure there is a Node with the proper label (cray.nnf.node=true), and
	// there is NNF Storage that contains that node as its one and only allocation.
	BeforeEach(func() {
		nodeKey = types.NamespacedName{
			Name: "test-node",
		}

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeKey.Name,
				Labels: map[string]string{
					"cray.nnf.node": "true",
				},
			},
		}
		Expect(k8sClient.Create(context.TODO(), node)).To(Succeed())

		storageKey = types.NamespacedName{
			Name:      "test-storage",
			Namespace: corev1.NamespaceDefault,
		}

		storage = &nnfv1alpha1.NnfStorage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      storageKey.Name,
				Namespace: storageKey.Namespace,
			},
			Spec: nnfv1alpha1.NnfStorageSpec{
				AllocationSets: []nnfv1alpha1.NnfStorageAllocationSetSpec{
					{
						Name:           "test-nnf-storage",
						Capacity:       0,
						FileSystemType: "lustre",
						Nodes: []nnfv1alpha1.NnfStorageAllocationNodes{
							{
								Name:  node.Name,
								Count: 1,
							},
						},
					},
				},
			},
		}
	})

	// After each test delete the NNF Storage and the Node
	AfterEach(func() {
		storage := &nnfv1alpha1.NnfStorage{}
		Expect(k8sClient.Get(context.TODO(), storageKey, storage)).To(Succeed())
		Expect(k8sClient.Delete(context.TODO(), storage)).To(Succeed())

		node := &corev1.Node{}
		Expect(k8sClient.Get(context.TODO(), nodeKey, node)).To(Succeed())
		Expect(k8sClient.Delete(context.TODO(), node)).To(Succeed())
	})

	// Just before each test, ensure the NNF Storage resource is created. This is
	// outside the BeforeEach() declartion so each test can modify the NNF Storage
	// resource as needed.
	JustBeforeEach(func() {
		Expect(k8sClient.Create(context.TODO(), storage)).To(Succeed())
		Eventually(func() error {
			return k8sClient.Get(context.TODO(), storageKey, storage)
		}).Should(Succeed(), "create the nnf storage resource")
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

	Describe("Lustre File System", func() {

		BeforeEach(func() {
			storage.Spec.AllocationSets[0].FileSystemType = "lustre"

			dm.Spec.Source = dmv1alpha1.DataMovementSpecSourceDestination{
				Path: "/lus/maui/example.file",
				StorageInstance: &corev1.ObjectReference{
					Kind:      "LustreFileSystem",
					Name:      "lustre-maui",
					Namespace: corev1.NamespaceDefault,
				},
			}

			dm.Spec.Destination = dmv1alpha1.DataMovementSpecSourceDestination{
				Path: "/mnt/lus/maui",
				StorageInstance: &corev1.ObjectReference{
					Kind:      "JobStorageInstance",
					Name:      "job-storage-instance-test",
					Namespace: corev1.NamespaceDefault,
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

		Describe("Check Lifecycle", func() {

			// After each life-cycle test specification, delete the Data Movement resource
			AfterEach(func() {
				expected := &dmv1alpha1.DataMovement{}
				Expect(k8sClient.Get(context.TODO(), dmKey, expected)).To(Succeed())
				Expect(k8sClient.Delete(context.TODO(), expected)).To(Succeed())
			})

			PIt("Labels the node", func() {
				Eventually(func() map[string]string {
					node := &corev1.Node{}
					Expect(k8sClient.Get(context.TODO(), nodeKey, node)).To(Succeed())
					return node.Labels
				}).Should(HaveKeyWithValue(dmKey.Name, "true"))
			})

			PIt("Creates the MPI Job", func() {
				Eventually(func() error {
					mpi := &mpiv2beta1.MPIJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:      dmKey.Name + mpiJobSuffix,
							Namespace: dmKey.Namespace,
						},
					}
					return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(mpi), mpi)
				}).Should(Succeed())
			})
		})

		Describe("Check delete operation", func() {

			PIt("Unlabels the node on delete", func() {
				Expect(k8sClient.Delete(context.TODO(), dm)).To(Succeed())

				Eventually(func() map[string]string {
					node := &corev1.Node{}
					Expect(k8sClient.Get(context.TODO(), nodeKey, node)).To(Succeed())
					return node.Labels
				}).ShouldNot(HaveKey(dmKey.Name))
			})
		})

		Context("Source is Global Lustre", func() {

			BeforeEach(func() {
				dm.Spec.Source = dmv1alpha1.DataMovementSpecSourceDestination{
					Path: "/lus/maui/myfile.tar.gz",
					StorageInstance: &corev1.ObjectReference{
						Kind:      "LustreFileSystem",
						Name:      "lustre-maui",
						Namespace: corev1.NamespaceDefault,
					},
				}
			})

			Describe("Destination is Job Storage Instance", func() {

				BeforeEach(func() {
					dm.Spec.Destination = dmv1alpha1.DataMovementSpecSourceDestination{
						Path: "/mnt/lus/maui",
						StorageInstance: &corev1.ObjectReference{
							Kind:      "JobStorageInstance",
							Name:      "job-storage-instance-test",
							Namespace: corev1.NamespaceDefault,
						},
					}
				})

				PIt("Creates PV/PVC", func() {
					Eventually(func() error {
						pv := &corev1.PersistentVolume{
							ObjectMeta: metav1.ObjectMeta{
								Name:      dmKey.Name + persistentVolumeSuffix,
								Namespace: dmKey.Namespace,
							},
						}
						return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(pv), pv)
					}).Should(Succeed())

					Eventually(func() error {
						pvc := &corev1.PersistentVolumeClaim{
							ObjectMeta: metav1.ObjectMeta{
								Name:      dmKey.Name + persistentVolumeClaimSuffix,
								Namespace: dmKey.Namespace,
							},
						}
						return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(pvc), pvc)
					}).Should(Succeed())
				})
			})

			Describe("Destination is Persistent Storage Instance", func() {

				BeforeEach(func() {
					dm.Spec.Destination = dmv1alpha1.DataMovementSpecSourceDestination{
						Path: "/mnt/lus/maui",
						StorageInstance: &corev1.ObjectReference{
							Kind:      "PersistentStorageInstance",
							Name:      "persistent-storage-instance-test",
							Namespace: corev1.NamespaceDefault,
						},
					}
				})
			})

		})

		Context("Source is Persistent Lustre", func() {

		})

	})

})

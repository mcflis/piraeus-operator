package v1_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	piraeusv1 "github.com/piraeusdatastore/piraeus-operator/v2/api/v1"
)

var _ = Describe("LinstorSatelliteConfiguration webhook", func() {
	typeMeta := metav1.TypeMeta{
		Kind:       "LinstorSatelliteConfiguration",
		APIVersion: piraeusv1.GroupVersion.String(),
	}
	complexSatelliteConfig := &piraeusv1.LinstorSatelliteConfiguration{
		TypeMeta:   typeMeta,
		ObjectMeta: metav1.ObjectMeta{Name: "example-nodes"},
		Spec: piraeusv1.LinstorSatelliteConfigurationSpec{
			NodeSelector: map[string]string{
				"kubernetes.io/hostname": "node-1.example.com",
			},
			Patches: []piraeusv1.Patch{
				{
					Target: &piraeusv1.Selector{
						Name: "satellite",
						Kind: "Pod",
					},
					Patch: "apiVersion: v1\nkind: Pod\nmetadata:\n  name: satellite\n  annotations:\n    k8s.v1.cni.cncf.io/networks: eth1",
				},
			},
			StoragePools: []piraeusv1.LinstorStoragePool{
				{
					Name: "thinpool",
					LvmThinPool: &piraeusv1.LinstorStoragePoolLvmThin{
						VolumeGroup: "linstor_thinpool",
						ThinPool:    "thinpool",
					},
					Source: &piraeusv1.LinstorStoragePoolSource{
						HostDevices: []string{"/dev/vdb"},
					},
				},
			},
			Properties: []piraeusv1.LinstorNodeProperty{
				{
					Name:      "Aux/topology/linbit.com/hostname",
					ValueFrom: &piraeusv1.LinstorNodePropertyValueFrom{NodeFieldRef: "metadata.name"},
				},
				{
					Name:      "Aux/topology/kubernetes.io/hostname",
					ValueFrom: &piraeusv1.LinstorNodePropertyValueFrom{NodeFieldRef: "metadata.labels['kubernetes.io/hostname']"},
				},
				{
					Name:      "Aux/topology/topology.kubernetes.io/region",
					ValueFrom: &piraeusv1.LinstorNodePropertyValueFrom{NodeFieldRef: "metadata.labels['topology.kubernetes.io/region']"},
				},
				{
					Name:      "Aux/topology/topology.kubernetes.io/zone",
					ValueFrom: &piraeusv1.LinstorNodePropertyValueFrom{NodeFieldRef: "metadata.labels['topology.kubernetes.io/zone']"},
				},
				{
					Name:  "PrefNic",
					Value: "default-ipv4",
				},
			},
		},
	}

	AfterEach(func(ctx context.Context) {
		err := k8sClient.DeleteAllOf(ctx, &piraeusv1.LinstorSatelliteConfiguration{})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should allow empty satellite configuration", func(ctx context.Context) {
		satelliteConfig := &piraeusv1.LinstorSatelliteConfiguration{TypeMeta: typeMeta, ObjectMeta: metav1.ObjectMeta{Name: "all-satellites"}}
		err := k8sClient.Patch(ctx, satelliteConfig, client.Apply, client.FieldOwner("test"), client.ForceOwnership)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should allow complex satellite", func(ctx context.Context) {
		err := k8sClient.Patch(ctx, complexSatelliteConfig.DeepCopy(), client.Apply, client.FieldOwner("test"), client.ForceOwnership)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should allow updating a complex satellite", func(ctx context.Context) {
		err := k8sClient.Patch(ctx, complexSatelliteConfig.DeepCopy(), client.Apply, client.FieldOwner("test"), client.ForceOwnership)
		Expect(err).NotTo(HaveOccurred())

		satelliteConfigCopy := complexSatelliteConfig.DeepCopy()
		satelliteConfigCopy.Spec.Patches[0].Patch = "apiVersion: v1\nkind: Pod\nmetadata:\n  name: satellite\n  annotations:\n    k8s.v1.cni.cncf.io/networks: eth1\nspec:\n  containers:\n  - name: linstor-satellite\n    volumeMounts:\n    - name: var-lib-drbd\n      mountPath: /var/lib/drbd\n  volumes:\n  - name: var-lib-drbd\n    emptyDir: {}\n"
		err = k8sClient.Patch(ctx, satelliteConfigCopy, client.Apply, client.FieldOwner("test"), client.ForceOwnership)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should require exactly one pool type for storage pools", func(ctx context.Context) {
		satelliteConfig := &piraeusv1.LinstorSatelliteConfiguration{
			TypeMeta:   typeMeta,
			ObjectMeta: metav1.ObjectMeta{Name: "storage-pools"},
			Spec: piraeusv1.LinstorSatelliteConfigurationSpec{
				StoragePools: []piraeusv1.LinstorStoragePool{
					{Name: "missing-type"},
					{Name: "multiple-types", LvmPool: &piraeusv1.LinstorStoragePoolLvm{}, LvmThinPool: &piraeusv1.LinstorStoragePoolLvmThin{}},
					{Name: "valid-pool", LvmPool: &piraeusv1.LinstorStoragePoolLvm{}},
				},
			},
		}
		err := k8sClient.Patch(ctx, satelliteConfig, client.Apply, client.FieldOwner("test"), client.ForceOwnership)
		Expect(err).To(HaveOccurred())
		statusErr := err.(*errors.StatusError)
		Expect(statusErr).NotTo(BeNil())
		Expect(statusErr.ErrStatus.Details).NotTo(BeNil())
		Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(2))
		Expect(statusErr.ErrStatus.Details.Causes[0].Field).To(Equal("spec.storagePools.0"))
		Expect(statusErr.ErrStatus.Details.Causes[1].Field).To(Equal("spec.storagePools.1.lvm"))
	})

	It("should reject improper node selectors", func(ctx context.Context) {
		satelliteConfig := &piraeusv1.LinstorSatelliteConfiguration{
			TypeMeta:   typeMeta,
			ObjectMeta: metav1.ObjectMeta{Name: "invalid-labels"},
			Spec: piraeusv1.LinstorSatelliteConfigurationSpec{
				NodeSelector: map[string]string{
					"example.com/key1":           "valid-label",
					"12.34.not+a+valid+key/key1": "valid-value",
					"example.com/key2":           "not a valid value",
				},
			},
		}
		err := k8sClient.Patch(ctx, satelliteConfig, client.Apply, client.FieldOwner("test"), client.ForceOwnership)
		Expect(err).To(HaveOccurred())
		statusErr := err.(*errors.StatusError)
		Expect(statusErr).NotTo(BeNil())
		Expect(statusErr.ErrStatus.Details).NotTo(BeNil())
		Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(2))
	})
})

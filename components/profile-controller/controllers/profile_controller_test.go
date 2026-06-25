package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	profilev1 "github.com/kubeflow/dashboard/components/profile-controller/api/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// =========================================================================
// Existing tests (standard Go testing.T style)
// =========================================================================

type namespaceLabelSuite struct {
	current  corev1.Namespace
	labels   map[string]string
	expected corev1.Namespace
}

func TestEnforceNamespaceLabelsFromConfig(t *testing.T) {
	name := "test-namespace"
	tests := []namespaceLabelSuite{
		{
			corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
			},
			map[string]string{
				"katib.kubeflow.org/metrics-collector-injection": "enabled",
				"serving.kubeflow.org/inferenceservice":          "enabled",
				"pipelines.kubeflow.org/enabled":                 "true",
				"app.kubernetes.io/part-of":                      "kubeflow-profile",
			},
			corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"katib.kubeflow.org/metrics-collector-injection": "enabled",
						"serving.kubeflow.org/inferenceservice":          "enabled",
						"pipelines.kubeflow.org/enabled":                 "true",
						"app.kubernetes.io/part-of":                      "kubeflow-profile",
					},
					Name: name,
				},
			},
		},
		{
			corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"user-name":                             "Jim",
						"serving.kubeflow.org/inferenceservice": "disabled",
					},
					Name: name,
				},
			},
			map[string]string{
				"katib.kubeflow.org/metrics-collector-injection": "enabled",
				"serving.kubeflow.org/inferenceservice":          "enabled",
				"pipelines.kubeflow.org/enabled":                 "true",
				"app.kubernetes.io/part-of":                      "kubeflow-profile",
			},
			corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"user-name": "Jim",
						"katib.kubeflow.org/metrics-collector-injection": "enabled",
						"serving.kubeflow.org/inferenceservice":          "disabled",
						"pipelines.kubeflow.org/enabled":                 "true",
						"app.kubernetes.io/part-of":                      "kubeflow-profile",
					},
					Name: name,
				},
			},
		},
		{
			corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"user-name":     "Jim",
						"removal-label": "enabled",
					},
					Name: name,
				},
			},
			map[string]string{
				"katib.kubeflow.org/metrics-collector-injection": "enabled",
				"serving.kubeflow.org/inferenceservice":          "enabled",
				"pipelines.kubeflow.org/enabled":                 "true",
				"app.kubernetes.io/part-of":                      "kubeflow-profile",
				"removal-label":                                  "",
			},
			corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"user-name": "Jim",
						"katib.kubeflow.org/metrics-collector-injection": "enabled",
						"serving.kubeflow.org/inferenceservice":          "enabled",
						"pipelines.kubeflow.org/enabled":                 "true",
						"app.kubernetes.io/part-of":                      "kubeflow-profile",
					},
					Name: name,
				},
			},
		},
	}
	for _, test := range tests {
		setNamespaceLabels(&test.current, test.labels)
		if !reflect.DeepEqual(&test.expected, &test.current) {
			t.Errorf("Expect:\n%v; Output:\n%v", &test.expected, &test.current)
		}
	}
}

type getPluginSpecSuite struct {
	profile         *profilev1.Profile
	expectedPlugins []Plugin
}

func TestGetPluginSpec(t *testing.T) {
	role_arn := "arn:aws:iam::123456789012:role/test-iam-role"
	gcp_sa := "kubeflow2@project-id.iam.gserviceaccount.com"
	tests := []getPluginSpecSuite{
		{
			&profilev1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-user-profile",
					Namespace: "k8snamespace",
				},
				Spec: profilev1.ProfileSpec{
					Plugins: []profilev1.Plugin{
						{
							TypeMeta: metav1.TypeMeta{
								Kind: KIND_AWS_IAM_FOR_SERVICE_ACCOUNT,
							},
							Spec: &runtime.RawExtension{
								Raw: []byte(fmt.Sprintf(`{"awsIamRole": "%v"}`, role_arn)),
							},
						},
					},
				},
			},
			[]Plugin{
				&AwsIAMForServiceAccount{
					AwsIAMRole: role_arn,
				},
			},
		},
		{
			&profilev1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gcp-user-profile",
					Namespace: "k8snamespace",
				},
				Spec: profilev1.ProfileSpec{
					Plugins: []profilev1.Plugin{
						{
							TypeMeta: metav1.TypeMeta{
								Kind: KIND_WORKLOAD_IDENTITY,
							},
							Spec: &runtime.RawExtension{
								Raw: []byte(fmt.Sprintf(`{"gcpServiceAccount": "%v"}`, gcp_sa)),
							},
						},
					},
				},
			},
			[]Plugin{
				&GcpWorkloadIdentity{
					GcpServiceAccount: gcp_sa,
				},
			},
		},
	}
	for _, test := range tests {
		loadedPlugins, err := createMockReconciler().GetPluginSpec(test.profile)

		assert.Nil(t, err)
		if !reflect.DeepEqual(&test.expectedPlugins, &loadedPlugins) {
			expected, _ := json.Marshal(test.expectedPlugins)
			found, _ := json.Marshal(loadedPlugins)
			t.Errorf("Test: %v. Expected:\n%v\nFound:\n%v", test.profile.Name, string(expected), string(found))
		}
	}
}

func createMockReconciler() *ProfileReconciler {
	reconciler := &ProfileReconciler{
		Scheme:                     runtime.NewScheme(),
		Log:                        ctrl.Log,
		UserIdHeader:               "dummy",
		UserIdPrefix:               "dummy",
		WorkloadIdentity:           "dummy",
		DefaultNamespaceLabelsPath: "dummy",
	}
	return reconciler
}

// =========================================================================
// New Ginkgo tests — Namespace Adoption
// =========================================================================

var _ = Describe("Profile Controller - Namespace Adoption", func() {

	const timeout = time.Second * 30
	const interval = time.Millisecond * 250

	ctx := context.Background()

	// ---------------------------------------------------------------
	// Helper: create a bare namespace (simulates kubectl create ns)
	// ---------------------------------------------------------------
	createBareNamespace := func(name string, annotations map[string]string, labels map[string]string) {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Annotations: annotations,
				Labels:      labels,
			},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
	}

	// ---------------------------------------------------------------
	// Helper: create a Profile CR
	// ---------------------------------------------------------------
	createProfile := func(name, owner string) *profilev1.Profile {
		profile := &profilev1.Profile{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: profilev1.ProfileSpec{
				Owner: rbacv1.Subject{
					Kind: "User",
					Name: owner,
				},
			},
		}
		Expect(k8sClient.Create(ctx, profile)).Should(Succeed())
		return profile
	}

	// ---------------------------------------------------------------
	// Test 1: Namespace with NO owner annotation — should be adopted
	// ---------------------------------------------------------------
	Context("When a namespace exists with no owner annotation", func() {
		var nsName = "adopt-no-owner"
		var ownerName = "alice@example.com"

		BeforeEach(func() {
			createBareNamespace(nsName, nil, nil)
		})

		AfterEach(func() {
			profile := &profilev1.Profile{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, profile); err == nil {
				Expect(k8sClient.Delete(ctx, profile)).Should(Succeed())
			}
			ns := &corev1.Namespace{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns); err == nil {
				Expect(k8sClient.Delete(ctx, ns)).Should(Succeed())
			}
		})

		It("should adopt the namespace and set the owner annotation", func() {
			createProfile(nsName, ownerName)

			Eventually(func() string {
				ns := &corev1.Namespace{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns); err != nil {
					return ""
				}
				return ns.Annotations["owner"]
			}, timeout, interval).Should(Equal(ownerName))
		})

		It("should apply Kubeflow namespace labels after adoption", func() {
			createProfile(nsName, ownerName)

			Eventually(func() bool {
				ns := &corev1.Namespace{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns); err != nil {
					return false
				}
				_, hasLabel := ns.Labels["app.kubernetes.io/part-of"]
				return hasLabel
			}, timeout, interval).Should(BeTrue())
		})

		It("should enable istio injection label on adopted namespace", func() {
			createProfile(nsName, ownerName)

			Eventually(func() string {
				ns := &corev1.Namespace{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns); err != nil {
					return ""
				}
				return ns.Labels["istio-injection"]
			}, timeout, interval).Should(Equal("enabled"))
		})
	})

	// ---------------------------------------------------------------
	// Test 2: Namespace already owned by the SAME user — update labels
	// ---------------------------------------------------------------
	Context("When a namespace exists already owned by the same user", func() {
		var nsName = "same-owner-ns"
		var ownerName = "bob@example.com"

		BeforeEach(func() {
			createBareNamespace(nsName,
				map[string]string{"owner": ownerName},
				nil,
			)
		})

		AfterEach(func() {
			profile := &profilev1.Profile{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, profile); err == nil {
				Expect(k8sClient.Delete(ctx, profile)).Should(Succeed())
			}
			ns := &corev1.Namespace{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns); err == nil {
				Expect(k8sClient.Delete(ctx, ns)).Should(Succeed())
			}
		})

		It("should update labels without changing the owner annotation", func() {
			createProfile(nsName, ownerName)

			Eventually(func() bool {
				ns := &corev1.Namespace{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns); err != nil {
					return false
				}
				_, hasLabel := ns.Labels["app.kubernetes.io/part-of"]
				return hasLabel
			}, timeout, interval).Should(BeTrue())

			// Owner annotation must remain unchanged
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns)).Should(Succeed())
			Expect(ns.Annotations["owner"]).To(Equal(ownerName))
		})
	})

	// ---------------------------------------------------------------
	// Test 3: Namespace owned by a DIFFERENT user — must be rejected
	// ---------------------------------------------------------------
	Context("When a namespace exists owned by a different user", func() {
		var nsName = "different-owner-ns"
		var existingOwner = "carol@example.com"
		var newOwner = "dave@example.com"

		BeforeEach(func() {
			createBareNamespace(nsName,
				map[string]string{"owner": existingOwner},
				nil,
			)
		})

		AfterEach(func() {
			profile := &profilev1.Profile{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, profile); err == nil {
				Expect(k8sClient.Delete(ctx, profile)).Should(Succeed())
			}
			ns := &corev1.Namespace{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns); err == nil {
				Expect(k8sClient.Delete(ctx, ns)).Should(Succeed())
			}
		})

		It("should NOT overwrite the existing owner annotation", func() {
			createProfile(nsName, newOwner)

			Consistently(func() string {
				ns := &corev1.Namespace{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns); err != nil {
					return ""
				}
				return ns.Annotations["owner"]
			}, time.Second*5, interval).Should(Equal(existingOwner))
		})
	})

	// ---------------------------------------------------------------
	// Test 4: Namespace with existing Istio label — must NOT overwrite
	// ---------------------------------------------------------------
	Context("When a namespace already has a custom istio-injection label", func() {
		var nsName = "custom-istio-ns"
		var ownerName = "eve@example.com"

		BeforeEach(func() {
			createBareNamespace(nsName,
				nil,
				map[string]string{"istio-injection": "disabled"},
			)
		})

		AfterEach(func() {
			profile := &profilev1.Profile{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, profile); err == nil {
				Expect(k8sClient.Delete(ctx, profile)).Should(Succeed())
			}
			ns := &corev1.Namespace{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns); err == nil {
				Expect(k8sClient.Delete(ctx, ns)).Should(Succeed())
			}
		})

		It("should preserve the existing istio-injection label value", func() {
			createProfile(nsName, ownerName)

			Eventually(func() string {
				ns := &corev1.Namespace{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns); err != nil {
					return ""
				}
				return ns.Labels["istio-injection"]
			}, timeout, interval).Should(Equal("disabled"))
		})
	})
})

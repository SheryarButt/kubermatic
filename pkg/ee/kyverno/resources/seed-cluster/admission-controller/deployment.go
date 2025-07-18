//go:build ee

/*
                  Kubermatic Enterprise Read-Only License
                         Version 1.0 ("KERO-1.0”)
                     Copyright © 2025 Kubermatic GmbH

   1.	You may only view, read and display for studying purposes the source
      code of the software licensed under this license, and, to the extent
      explicitly provided under this license, the binary code.
   2.	Any use of the software which exceeds the foregoing right, including,
      without limitation, its execution, compilation, copying, modification
      and distribution, is expressly prohibited.
   3.	THE SOFTWARE IS PROVIDED “AS IS”, WITHOUT WARRANTY OF ANY KIND,
      EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
      MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
      IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
      CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
      TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
      SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

   END OF TERMS AND CONDITIONS
*/

package admissioncontrollerresources

import (
	"fmt"

	kyverno "k8c.io/kubermatic/v2/pkg/ee/kyverno/resources/seed-cluster/common"
	"k8c.io/kubermatic/v2/pkg/resources"
	"k8c.io/kubermatic/v2/pkg/resources/registry"
	"k8c.io/reconciler/pkg/reconciling"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

// DeploymentReconciler returns the function to create and update the Kyverno admission controller deployment.
func DeploymentReconciler(data kyverno.KyvernoData) reconciling.NamedDeploymentReconcilerFactory {
	return func() (string, reconciling.DeploymentReconciler) {
		return kyverno.KyvernoAdmissionControllerDeploymentName, func(dep *appsv1.Deployment) (*appsv1.Deployment, error) {
			dep.Labels = kyverno.KyvernoLabels(kyverno.AdmissionControllerComponentNameLabel)

			dep.Spec.Replicas = resources.Int32(kyverno.KyvernoAdmissionControllerReplicas)
			dep.Spec.RevisionHistoryLimit = resources.Int32(10)
			dep.Spec.Strategy = appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxSurge:       &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "40%"},
				},
			}

			dep.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: kyverno.KyvernoSelectorLabels(kyverno.AdmissionControllerComponentNameLabel),
			}

			dep.Spec.Template.Labels = kyverno.KyvernoLabels(kyverno.AdmissionControllerComponentNameLabel)

			dep.Spec.Template.Spec.DNSPolicy = corev1.DNSClusterFirst
			dep.Spec.Template.Spec.ServiceAccountName = kyverno.KyvernoAdmissionControllerServiceAccountName

			dep.Spec.Template.Spec.Affinity = &corev1.Affinity{
				PodAntiAffinity: &corev1.PodAntiAffinity{
					PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
						{
							Weight: 1,
							PodAffinityTerm: corev1.PodAffinityTerm{
								LabelSelector: &metav1.LabelSelector{
									MatchExpressions: []metav1.LabelSelectorRequirement{
										{
											Key:      "app.kubernetes.io/component",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{kyverno.AdmissionControllerComponentNameLabel},
										},
									},
								},
								TopologyKey: "kubernetes.io/hostname",
							},
						},
					},
				},
			}

			dep.Spec.Template.Spec.Volumes = []corev1.Volume{
				{
					Name: "uc-admin-kubeconfig",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: resources.InternalUserClusterAdminKubeconfigSecretName,
						},
					},
				},
				{
					Name: "sigstore",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			}
			repository := registry.Must(data.RewriteImage(kyverno.KyvernoRegistry + "/kyvernopre"))
			dep.Spec.Template.Spec.InitContainers = []corev1.Container{
				{
					Name:            "kyverno-pre",
					Image:           repository + ":" + kyverno.KyvernoVersion,
					ImagePullPolicy: corev1.PullPolicy("IfNotPresent"),
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("10m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: ptr.To(false),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
						Privileged:             ptr.To(false),
						ReadOnlyRootFilesystem: ptr.To(true),
						RunAsNonRoot:           ptr.To(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Env: []corev1.EnvVar{
						{
							Name:  "KYVERNO_SERVICEACCOUNT_NAME",
							Value: kyverno.KyvernoAdmissionControllerServiceAccountName,
						},
						{
							Name:  "KYVERNO_ROLE_NAME",
							Value: kyverno.KyvernoAdmissionControllerRoleName,
						},
						{
							Name:  "INIT_CONFIG",
							Value: kyverno.KyvernoConfigMapName,
						},
						{
							Name:  "METRICS_CONFIG",
							Value: kyverno.KyvernoMetricsConfigMapName,
						},
						{
							Name: "KYVERNO_NAMESPACE",
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: "metadata.namespace",
								},
							},
						},
						{
							Name: "KYVERNO_POD_NAME",
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: "metadata.name",
								},
							},
						},
						{
							Name:  "KYVERNO_DEPLOYMENT",
							Value: kyverno.KyvernoAdmissionControllerDeploymentName,
						},
						{
							Name:  "KYVERNO_SVC",
							Value: kyverno.KyvernoAdmissionControllerServiceName,
						},
					},
				},
			}

			namespace := data.Cluster().Status.NamespaceName
			repository = registry.Must(data.RewriteImage(kyverno.KyvernoRegistry + "/kyverno"))
			dep.Spec.Template.Spec.Containers = []corev1.Container{
				{
					Name:            "kyverno",
					Image:           repository + ":" + kyverno.KyvernoVersion,
					ImagePullPolicy: corev1.PullPolicy("IfNotPresent"),
					Args: []string{
						"--kubeconfig=/etc/kubernetes/uc-admin-kubeconfig/kubeconfig",
						"--serverIP=kyverno-svc." + namespace + ".svc.cluster.local.",
						fmt.Sprintf("--caSecretName=kyverno-svc.%s.svc.kyverno-tls-ca", namespace),
						fmt.Sprintf("--tlsSecretName=kyverno-svc.%s.svc.kyverno-tls-pair", namespace),
						fmt.Sprintf("--backgroundServiceAccountName=system:serviceaccount:%s:kyverno-background-controller", namespace),
						fmt.Sprintf("--reportsServiceAccountName=system:serviceaccount:%s:kyverno-reports-controller", namespace),
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("384Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					SecurityContext: &corev1.SecurityContext{
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
						ReadOnlyRootFilesystem: ptr.To(true),
						RunAsNonRoot:           ptr.To(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Ports: []corev1.ContainerPort{
						{
							Name:          "https",
							ContainerPort: 9443,
							Protocol:      corev1.ProtocolTCP,
						},
						{
							Name:          "metrics-port",
							ContainerPort: 8000,
							Protocol:      corev1.ProtocolTCP,
						},
					},
					Env: []corev1.EnvVar{
						{
							Name:  "INIT_CONFIG",
							Value: kyverno.KyvernoConfigMapName,
						},
						{
							Name:  "METRICS_CONFIG",
							Value: kyverno.KyvernoMetricsConfigMapName,
						},
						{
							Name: "KYVERNO_NAMESPACE",
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: "metadata.namespace",
								},
							},
						},
						{
							Name: "KYVERNO_POD_NAME",
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: "metadata.name",
								},
							},
						},
						{
							Name:  "KYVERNO_SERVICEACCOUNT_NAME",
							Value: kyverno.KyvernoAdmissionControllerServiceAccountName,
						},
						{
							Name:  "KYVERNO_ROLE_NAME",
							Value: kyverno.KyvernoAdmissionControllerRoleName,
						},
						{
							Name:  "KYVERNO_SVC",
							Value: kyverno.KyvernoAdmissionControllerServiceName,
						},
						{
							Name:  "TUF_ROOT",
							Value: "/.sigstore",
						},
						{
							Name:  "KYVERNO_DEPLOYMENT",
							Value: kyverno.KyvernoAdmissionControllerDeploymentName,
						},
					},
					StartupProbe: &corev1.Probe{
						FailureThreshold: 20,
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path:   "/health/liveness",
								Port:   intstr.FromInt(9443),
								Scheme: corev1.URISchemeHTTPS,
							},
						},
						InitialDelaySeconds: 2,
						PeriodSeconds:       6,
						SuccessThreshold:    1,
						TimeoutSeconds:      1,
					},
					LivenessProbe: &corev1.Probe{
						FailureThreshold: 2,
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path:   "/health/liveness",
								Port:   intstr.FromInt(9443),
								Scheme: corev1.URISchemeHTTPS,
							},
						},
						InitialDelaySeconds: 15,
						PeriodSeconds:       30,
						SuccessThreshold:    1,
						TimeoutSeconds:      5,
					},
					ReadinessProbe: &corev1.Probe{
						FailureThreshold: 6,
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path:   "/health/readiness",
								Port:   intstr.FromInt(9443),
								Scheme: corev1.URISchemeHTTPS,
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       10,
						SuccessThreshold:    1,
						TimeoutSeconds:      5,
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "uc-admin-kubeconfig",
							MountPath: "/etc/kubernetes/uc-admin-kubeconfig",
						},
						{
							Name:      "sigstore",
							MountPath: "/.sigstore",
						},
					},
				},
			}

			return dep, nil
		}
	}
}

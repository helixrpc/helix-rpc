/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	orchestrationv1alpha1 "github.com/helixrpc/helix-operator/api/v1alpha1"
)

// HelixServiceReconciler reconciles a HelixService object
type HelixServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=orchestration.helixrpc.io,resources=helixservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=orchestration.helixrpc.io,resources=helixservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=orchestration.helixrpc.io,resources=helixservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *HelixServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	helix := &orchestrationv1alpha1.HelixService{}
	err := r.Get(ctx, req.NamespacedName, helix)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	dep := r.deploymentForHelix(helix)
	if err := ctrl.SetControllerReference(helix, dep, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: dep.Name, Namespace: dep.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.Create(ctx, dep)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Ensure eBPF bypass state is correct
	if helix.Spec.EnableEBPFBypass && !hasEBPFMount(found) {
		log.Info("Updating Deployment to inject eBPF mounts")
		err = r.Update(ctx, dep)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *HelixServiceReconciler) deploymentForHelix(m *orchestrationv1alpha1.HelixService) *appsv1.Deployment {
	ls := labelsForHelix(m.Name)
	replicas := m.Spec.Replicas
	if replicas == nil {
		r := int32(1)
		replicas = &r
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      ls,
					Annotations: map[string]string{},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: m.Spec.Image,
						Name:  "helix-runtime",
						Ports: []corev1.ContainerPort{{
							ContainerPort: 8080,
							Name:          "http",
						}},
					}},
				},
			},
		},
	}

	if m.Spec.VaultSecretPath != "" {
		dep.Spec.Template.ObjectMeta.Annotations["vault.hashicorp.com/agent-inject"] = "true"
		dep.Spec.Template.ObjectMeta.Annotations["vault.hashicorp.com/role"] = "helix-role"
		dep.Spec.Template.ObjectMeta.Annotations["vault.hashicorp.com/agent-inject-secret-helix-secrets.json"] = m.Spec.VaultSecretPath
	}

	if m.Spec.EnableEBPFBypass {
		hostPathType := corev1.HostPathDirectoryOrCreate
		dep.Spec.Template.Spec.Volumes = append(dep.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "sys-fs-bpf",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/sys/fs/bpf",
					Type: &hostPathType,
				},
			},
		})

		t := true
		dep.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
			Privileged: &t,
		}
		dep.Spec.Template.Spec.Containers[0].VolumeMounts = append(dep.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "sys-fs-bpf",
			MountPath: "/sys/fs/bpf",
		})
	}

	return dep
}

func hasEBPFMount(dep *appsv1.Deployment) bool {
	for _, v := range dep.Spec.Template.Spec.Volumes {
		if v.Name == "sys-fs-bpf" {
			return true
		}
	}
	return false
}

func labelsForHelix(name string) map[string]string {
	return map[string]string{"app": "helix-service", "helix_cr": name}
}

// SetupWithManager sets up the controller with the Manager.
func (r *HelixServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&orchestrationv1alpha1.HelixService{}).
		Owns(&appsv1.Deployment{}).
		Named("helixservice").
		Complete(r)
}

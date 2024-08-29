package controllers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *CloudCostOptimizerReconciler) updateDeployment(ctx context.Context, namespace, name string, recommendations []ResourceRecommendation) error {
	var deployment appsv1.Deployment
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &deployment); err != nil {
		return fmt.Errorf("failed to get Deployment: %v", err)
	}

	logger := log.FromContext(ctx)
	logger.Info("Updating Deployment", "deployment", deployment.Name)

	updated := false
	for i, container := range deployment.Spec.Template.Spec.Containers {
		for _, rec := range recommendations {
			if container.Name == rec.ContainerName {
				if deployment.Spec.Template.Spec.Containers[i].Resources.Requests == nil {
					deployment.Spec.Template.Spec.Containers[i].Resources.Requests = corev1.ResourceList{}
				}
				if deployment.Spec.Template.Spec.Containers[i].Resources.Limits == nil {
					deployment.Spec.Template.Spec.Containers[i].Resources.Limits = corev1.ResourceList{}
				}
				deployment.Spec.Template.Spec.Containers[i].Resources.Requests[corev1.ResourceCPU] = *rec.RecommendedCPU
				deployment.Spec.Template.Spec.Containers[i].Resources.Requests[corev1.ResourceMemory] = *rec.RecommendedMemory
				deployment.Spec.Template.Spec.Containers[i].Resources.Limits[corev1.ResourceCPU] = *rec.RecommendedCPU
				deployment.Spec.Template.Spec.Containers[i].Resources.Limits[corev1.ResourceMemory] = *rec.RecommendedMemory
				updated = true
			}
		}
	}

	if updated {
		if err := r.Update(ctx, &deployment); err != nil {
			return fmt.Errorf("failed to update Deployment: %v", err)
		}
	}

	return nil
}

func (r *CloudCostOptimizerReconciler) updateStatefulSet(ctx context.Context, namespace, name string, recommendations []ResourceRecommendation) error {
	var statefulSet appsv1.StatefulSet
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &statefulSet); err != nil {
		return fmt.Errorf("failed to get StatefulSet: %v", err)
	}

	updated := false
	for i, container := range statefulSet.Spec.Template.Spec.Containers {
		for _, rec := range recommendations {
			if container.Name == rec.ContainerName {
				if statefulSet.Spec.Template.Spec.Containers[i].Resources.Requests == nil {
					statefulSet.Spec.Template.Spec.Containers[i].Resources.Requests = corev1.ResourceList{}
				}
				if statefulSet.Spec.Template.Spec.Containers[i].Resources.Limits == nil {
					statefulSet.Spec.Template.Spec.Containers[i].Resources.Limits = corev1.ResourceList{}
				}
				statefulSet.Spec.Template.Spec.Containers[i].Resources.Requests[corev1.ResourceCPU] = *rec.RecommendedCPU
				statefulSet.Spec.Template.Spec.Containers[i].Resources.Requests[corev1.ResourceMemory] = *rec.RecommendedMemory
				statefulSet.Spec.Template.Spec.Containers[i].Resources.Limits[corev1.ResourceCPU] = *rec.RecommendedCPU
				statefulSet.Spec.Template.Spec.Containers[i].Resources.Limits[corev1.ResourceMemory] = *rec.RecommendedMemory
				updated = true
			}
		}
	}

	if updated {
		if err := r.Update(ctx, &statefulSet); err != nil {
			return fmt.Errorf("failed to update StatefulSet: %v", err)
		}
	}

	return nil
}

func (r *CloudCostOptimizerReconciler) updateDaemonSet(ctx context.Context, namespace, name string, recommendations []ResourceRecommendation) error {
	var daemonSet appsv1.DaemonSet
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &daemonSet); err != nil {
		return fmt.Errorf("failed to get DaemonSet: %v", err)
	}

	updated := false
	for i, container := range daemonSet.Spec.Template.Spec.Containers {
		for _, rec := range recommendations {
			if container.Name == rec.ContainerName {
				if daemonSet.Spec.Template.Spec.Containers[i].Resources.Requests == nil {
					daemonSet.Spec.Template.Spec.Containers[i].Resources.Requests = corev1.ResourceList{}
				}
				if daemonSet.Spec.Template.Spec.Containers[i].Resources.Limits == nil {
					daemonSet.Spec.Template.Spec.Containers[i].Resources.Limits = corev1.ResourceList{}
				}
				daemonSet.Spec.Template.Spec.Containers[i].Resources.Requests[corev1.ResourceCPU] = *rec.RecommendedCPU
				daemonSet.Spec.Template.Spec.Containers[i].Resources.Requests[corev1.ResourceMemory] = *rec.RecommendedMemory
				daemonSet.Spec.Template.Spec.Containers[i].Resources.Limits[corev1.ResourceCPU] = *rec.RecommendedCPU
				daemonSet.Spec.Template.Spec.Containers[i].Resources.Limits[corev1.ResourceMemory] = *rec.RecommendedMemory
				updated = true
			}
		}
	}

	if updated {
		if err := r.Update(ctx, &daemonSet); err != nil {
			return fmt.Errorf("failed to update DaemonSet: %v", err)
		}
	}

	return nil
}

// isPodCreatedByJobOrCronJob checks if a pod was created by a Job or CronJob
func isPodCreatedByJobOrCronJob(pod *corev1.Pod) (bool, string) {
	for _, ownerRef := range pod.OwnerReferences {
		if ownerRef.Kind == "Job" {
			return true, "Job"
		}
	}

	return false, ""
}

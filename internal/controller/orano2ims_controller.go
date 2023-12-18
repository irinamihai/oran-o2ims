/*
Copyright 2023.

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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sptr "k8s.io/utils/ptr"

	oranv1alpha1 "oran-o2ims/api/v1alpha1"
	"oran-o2ims/internal/controller/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// OranO2IMSReconciler reconciles a OranO2IMS object
type OranO2IMSReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=oran.openshift.io,resources=orano2ims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=oran.openshift.io,resources=orano2ims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=oran.openshift.io,resources=orano2ims/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the OranO2IMS object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *OranO2IMSReconciler) Reconcile(ctx context.Context, req ctrl.Request) (nextReconcile ctrl.Result, err error) {
	orano2ims := &oranv1alpha1.OranO2IMS{}
	if err := r.Get(ctx, req.NamespacedName, orano2ims); err != nil {
		r.Log.Error(err, "Unable to fetch Oran")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	r.Log.Info(">>> Got orano2ims", "Spec.CloudId", orano2ims.Spec.CloudId)
	r.Log.Info(">>> Got orano2ims", "Spec.MetadataServer", orano2ims.Spec.MetadataServer)
	nextReconcile = ctrl.Result{RequeueAfter: 5 * time.Second}

	// Start the metadata server if required.
	if orano2ims.Spec.MetadataServer == true {
		// Create the needed ServiceAccount.
		err = r.createServiceAccount(ctx, utils.ORANO2IMSMetadataServerName, utils.ORANO2IMSNamespace)
		if err != nil {
			r.Log.Error(err, "Failed to deploy ServiceAccount for Metadata server.")
			return
		}

		// Create the metadata-server deployment.
		err = r.deployMetadataServer(ctx, orano2ims)
		if err != nil {
			r.Log.Error(err, "Failed to deploy the Metadata server.")
			return
		}
	}

	return
}

func (r *OranO2IMSReconciler) deployMetadataServer(ctx context.Context, orano2ims *oranv1alpha1.OranO2IMS) error {
	r.Log.Info("[deployMetadataServer]")

	// Check if the Deployment already exists.
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx,
		types.NamespacedName{Name: utils.ORANO2IMSMetadataServerName, Namespace: utils.ORANO2IMSNamespace},
		deployment)

	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("[deployMetadataServer] Deployment not found, create it")
		} else {
			return err
		}
	} else {
		r.Log.Info("[deployMetadataServer] Deployment already present, return")
		return nil
	}

	// Build the deployment's metadata.
	deploymentMeta := metav1.ObjectMeta{
		Name:      utils.ORANO2IMSMetadataServerName,
		Namespace: utils.ORANO2IMSNamespace,
		Labels: map[string]string{
			"oran/o2ims": orano2ims.Name,
			"app": utils.ORANO2IMSMetadataServerName,
		},
	}

	// Build the deployment's spec.
	deploymentSpec := appsv1.DeploymentSpec{
		Replicas: k8sptr.To(int32(1)),
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": utils.ORANO2IMSMetadataServerName,
			},
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"app": utils.ORANO2IMSMetadataServerName,
				},
			},
			Spec: corev1.PodSpec{
				ServiceAccountName: utils.ORANO2IMSMetadataServerName,
				Volumes: []corev1.Volume{
					{
						Name: "tls",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: utils.ORANO2IMSMetadataServerSecret,
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:            "server",
						Image:           "quay.io/jhernand/o2ims:latest",
						ImagePullPolicy: "Always",
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "tls",
								MountPath: "/secrets/tls",
							},
						},
						Command: []string{"/usr/bin/oran-o2ims"},
						Args: []string{
							"start",
							"metadata-server",
							" --log-level=debug",
							" --log-file=stdout",
							" --api-listener-address=0.0.0.0:8000",
							" --api-listener-tls-crt=/secrets/tls/tls.crt",
							" --api-listener-tls-key=/secrets/tls/tls.key",
							fmt.Sprintf(" --cloud-id=%s", orano2ims.CreationTimestamp.Time),
							fmt.Sprintf(" --external-address=https://o2ims.%s", "ingress"), // TODO update this once Ingress is created
						},
						Ports: []corev1.ContainerPort{
							{
								Name:          "api",
								Protocol:      corev1.ProtocolTCP,
								ContainerPort: 8080,
							},
						},
					},
				},
			},
		},
	}

	// Build the deployment.
	newDeployment := &appsv1.Deployment{
		ObjectMeta: deploymentMeta,
		Spec:       deploymentSpec,
	}
	
	return r.Client.Create(ctx, newDeployment)
}

func (r *OranO2IMSReconciler) createServiceAccount(ctx context.Context, saName string, saNamespace string) error {
	r.Log.Info("[createServiceAccount]")
	// Build the ServiceAccount object.
	serviceAccountMeta := metav1.ObjectMeta{
		Name:      saName,
		Namespace: saNamespace,
		Annotations: map[string]string{
			"service.beta.openshift.io/serving-cert-secret-name": fmt.Sprintf("%s-tls", saName),
		},
	}

	newServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: serviceAccountMeta,
	}

	// Check if the ServiceAccount already exists.
	serviceAccount := &corev1.ServiceAccount{}
	err := r.Get(ctx, types.NamespacedName{Name: saName, Namespace: saNamespace}, serviceAccount)

	if err != nil {
		if errors.IsNotFound(err) {
			err = r.Client.Create(ctx, newServiceAccount)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return nil

}

func (r *OranO2IMSReconciler) createService(ctx context.Context, serviceName string, serviceNamespace string) error {
	r.Log.Info("[createServiceAccount]")
	// Build the Service object.
	serviceMeta := metav1.ObjectMeta{
		Name:      serviceName,
		Namespace: serviceNamespace,
		Labels: map[string]string{
			"app": serviceName,
		},
		Annotations: map[string]string{
			"service.beta.openshift.io/serving-cert-secret-name": fmt.Sprintf("%s-tls", serviceName),
		},
	}

	serviceSpec := corev1.ServiceSpec{
		Selector: map[string]string{
			"app": serviceName,
		},
		Ports: []corev1.ServicePort{
			{
				Name: "api",
				Port: 8080,
				TargetPort: intstr.FromString("api"),
			},
		},
	}

	newService := &corev1.Service{
		ObjectMeta: serviceMeta,
		Spec: serviceSpec,
	}

	// Check if the Service already exists.
	service := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: serviceNamespace}, service)

	if err != nil {
		if errors.IsNotFound(err) {
			err = r.Client.Create(ctx, newService)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *OranO2IMSReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		Named("orano2ims").
		For(&oranv1alpha1.OranO2IMS{},
			// Watch for create event for orano2ims.
			builder.WithPredicates(predicate.Funcs{
				UpdateFunc: func(e event.UpdateEvent) bool {
					// Generation is only updated on spec changes (also on deletion),
					// not metadata or status.
					oldGeneration := e.ObjectOld.GetGeneration()
					newGeneration := e.ObjectNew.GetGeneration()
					// spec update only for CGU
					return oldGeneration != newGeneration
				},
				CreateFunc:  func(e event.CreateEvent) bool { return true },
				GenericFunc: func(e event.GenericEvent) bool { return false },
				DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			})).
		Owns(&appsv1.Deployment{},
			// Watch for delete event for owned Deployments.
			builder.WithPredicates(predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return false },
				CreateFunc:  func(e event.CreateEvent) bool { return false },
				DeleteFunc:  func(e event.DeleteEvent) bool { return true },
				UpdateFunc:  func(e event.UpdateEvent) bool { return false },
			})).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}

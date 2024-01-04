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
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

	// Create the needed Ingress if at least one server is required by the Spec.
	if orano2ims.Spec.MetadataServer || orano2ims.Spec.DeploymentManagerServer {
		err = r.createIngress(ctx, orano2ims)
		if err != nil {
			r.Log.Error(err, "Failed to deploy Service for Metadata server.")
			return
		}
	}

	// Start the metadata server if required by the Spec.
	if orano2ims.Spec.MetadataServer {
		// Create the needed ServiceAccount.
		err = r.createServiceAccount(ctx, orano2ims, utils.ORANO2IMSMetadataServerName)
		if err != nil {
			r.Log.Error(err, "Failed to deploy ServiceAccount for Metadata server.")
			return
		}

		// Create the Service needed for the Metadata server.
		err = r.createService(ctx, orano2ims, utils.ORANO2IMSMetadataServerName)
		if err != nil {
			r.Log.Error(err, "Failed to deploy Service for Metadata server.")
			return
		}

		// Create the metadata-server deployment.
		err = r.deployMetadataServer(ctx, orano2ims)
		if err != nil {
			r.Log.Error(err, "Failed to deploy the Metadata server.")
			return
		}
	}

	// Start the deployment server if required by the Spec.
	if orano2ims.Spec.DeploymentManagerServer {
		// Create the needed ServiceAccount.
		err = r.createServiceAccount(ctx, orano2ims, utils.ORANO2IMSDeploymentManagerServerName)
		if err != nil {
			r.Log.Error(err, "Failed to deploy ServiceAccount for Deployment Manager server.")
			return
		}

		// Create the Service needed for the Metadata server.
		err = r.createService(ctx, orano2ims, utils.ORANO2IMSMetadataServerName)
		if err != nil {
			r.Log.Error(err, "Failed to deploy Service for Deployment Manager server.")
			return
		}

		// Create the metadata-server deployment.
		err = r.deployDeploymentManagerServer(ctx, orano2ims)
		if err != nil {
			r.Log.Error(err, "Failed to deploy the Deployment Manager server.")
			return
		}
	}

	return
}

func (r *OranO2IMSReconciler) deployMetadataServer(ctx context.Context, orano2ims *oranv1alpha1.OranO2IMS) error {
	r.Log.Info("[deployMetadataServer]")

	// Build the deployment's metadata.
	deploymentMeta := metav1.ObjectMeta{
		Name:      utils.ORANO2IMSMetadataServerName,
		Namespace: utils.ORANO2IMSNamespace,
		Labels: map[string]string{
			"oran/o2ims": orano2ims.Name,
			"app":        utils.ORANO2IMSMetadataServerName,
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
								SecretName: fmt.Sprintf("%s-tls", utils.ORANO2IMSMetadataServerName),
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:            "server",
						Image:           utils.ORANImage,
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
							"--log-level=debug",
							"--log-file=stdout",
							"--api-listener-address=0.0.0.0:8000",
							"--api-listener-tls-crt=/secrets/tls/tls.crt",
							"--api-listener-tls-key=/secrets/tls/tls.key",
							fmt.Sprintf("--cloud-id=%s", orano2ims.Spec.CloudId),
							fmt.Sprintf("--external-address=https://%s", orano2ims.Spec.IngressHost),
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

	// Set owner reference.
	if err := controllerutil.SetControllerReference(orano2ims, newDeployment, r.Scheme); err != nil {
		return err
	}

	// Check if the Deployment already exists.
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx,
		types.NamespacedName{Name: utils.ORANO2IMSMetadataServerName, Namespace: utils.ORANO2IMSNamespace},
		deployment)

	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("[deployMetadataServer] Metadata Deployment not found, create it")
			r.Client.Create(ctx, newDeployment)
		} else {
			return err
		}
	} else {
		r.Log.Info("[deployMetadataServer] Deployment already present, update it")
		newDeployment.SetResourceVersion(deployment.GetResourceVersion())
		return r.Client.Update(ctx, newDeployment)
	}

	return nil
}

func (r *OranO2IMSReconciler) deployDeploymentManagerServer(ctx context.Context, orano2ims *oranv1alpha1.OranO2IMS) error {
	r.Log.Info("[deployMetadataServer]")

	// Build the deployment's metadata.
	deploymentMeta := metav1.ObjectMeta{
		Name:      utils.ORANO2IMSDeploymentManagerServerName,
		Namespace: utils.ORANO2IMSNamespace,
		Labels: map[string]string{
			"oran/o2ims": orano2ims.Name,
			"app":        utils.ORANO2IMSDeploymentManagerServerName,
		},
	}

	// Build the deployment's spec.
	deploymentSpec := appsv1.DeploymentSpec{
		Replicas: k8sptr.To(int32(1)),
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": utils.ORANO2IMSDeploymentManagerServerName,
			},
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"app": utils.ORANO2IMSDeploymentManagerServerName,
				},
			},
			Spec: corev1.PodSpec{
				ServiceAccountName: utils.ORANO2IMSDeploymentManagerServerName,
				Volumes: []corev1.Volume{
					{
						Name: "tls",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: fmt.Sprintf("%s-tls", utils.ORANO2IMSDeploymentManagerServerName),
							},
						},
					},
					{
						Name: "authz",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: "authz"},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:            "server",
						Image:           utils.ORANImage,
						ImagePullPolicy: "Always",
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "tls",
								MountPath: "/secrets/tls",
							},
							{
								Name:      "authz",
								MountPath: "/configmaps/authz",
							},
						},
						Command: []string{"/usr/bin/oran-o2ims"},
						Args: []string{
							"start",
							"metadata-server",
							"--log-level=debug",
							"--log-file=stdout",
							"--api-listener-address=0.0.0.0:8000",
							"--api-listener-tls-crt=/secrets/tls/tls.crt",
							"--api-listener-tls-key=/secrets/tls/tls.key",
							"--authn-jwks-url=https://kubernetes.default.svc/openid/v1/jwks",
							"--authn-jwks-token-file=/run/secrets/kubernetes.io/serviceaccount/token",
							"--authn-jwks-ca-file=/run/secrets/kubernetes.io/serviceaccount/ca.crt",
							"--authz-acl-file=/configmaps/authz/acl.yaml",
							fmt.Sprintf("--cloud-id=%s", orano2ims.CreationTimestamp.Time),
							fmt.Sprintf("--backend-url=%s", orano2ims.Spec.BackendURL),
							fmt.Sprintf("--backend-token=%s", orano2ims.Spec.BackendToken),
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

	// Set owner reference.
	if err := controllerutil.SetControllerReference(orano2ims, newDeployment, r.Scheme); err != nil {
		return err
	}

	// Check if the Deployment already exists.
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx,
		types.NamespacedName{Name: utils.ORANO2IMSDeploymentManagerServerName, Namespace: utils.ORANO2IMSNamespace},
		deployment)

	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("[deployMetadataServer] Manager Deployment not found, create it")
			return r.Client.Create(ctx, newDeployment)
		} else {
			return err
		}
	} else {
		r.Log.Info("[deployMetadataServer] Manager Deployment already present, update it")
		newDeployment.SetResourceVersion(deployment.GetResourceVersion())
		return r.Client.Update(ctx, newDeployment)
	}
}

func (r *OranO2IMSReconciler) createServiceAccount(ctx context.Context, orano2ims *oranv1alpha1.OranO2IMS, resourceName string) error {
	r.Log.Info("[createServiceAccount]")
	// Build the ServiceAccount object.
	serviceAccountMeta := metav1.ObjectMeta{
		Name:      resourceName,
		Namespace: orano2ims.Namespace,
		Annotations: map[string]string{
			"service.beta.openshift.io/serving-cert-secret-name": fmt.Sprintf("%s-tls", orano2ims.Name),
		},
	}

	newServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: serviceAccountMeta,
	}

	// Set owner reference.
	if err := controllerutil.SetControllerReference(orano2ims, newServiceAccount, r.Scheme); err != nil {
		return err
	}

	// Check if the ServiceAccount already exists.
	serviceAccount := &corev1.ServiceAccount{}
	err := r.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: orano2ims.Namespace}, serviceAccount)

	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("[createServiceAccount] ServiceAccount not found, create it")
			err = r.Client.Create(ctx, newServiceAccount)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		r.Log.Info("[createServiceAccount] ServiceAccount already present, update it")
		newServiceAccount.SetResourceVersion(serviceAccount.GetResourceVersion())
		return r.Client.Update(ctx, newServiceAccount)
	}

	return nil
}

func (r *OranO2IMSReconciler) createService(ctx context.Context, orano2ims *oranv1alpha1.OranO2IMS, resourceName string) error {
	r.Log.Info("[createService]")
	// Build the Service object.
	serviceMeta := metav1.ObjectMeta{
		Name:      resourceName,
		Namespace: orano2ims.Namespace,
		Labels: map[string]string{
			"app": resourceName,
		},
		Annotations: map[string]string{
			"service.beta.openshift.io/serving-cert-secret-name": fmt.Sprintf("%s-tls", resourceName),
		},
	}

	serviceSpec := corev1.ServiceSpec{
		Selector: map[string]string{
			"app": resourceName,
		},
		Ports: []corev1.ServicePort{
			{
				Name:       "api",
				Port:       8000,
				TargetPort: intstr.FromString("api"),
			},
		},
	}

	newService := &corev1.Service{
		ObjectMeta: serviceMeta,
		Spec:       serviceSpec,
	}

	// Set owner reference.
	if err := controllerutil.SetControllerReference(orano2ims, newService, r.Scheme); err != nil {
		return err
	}

	// Check if the Service already exists.
	service := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: orano2ims.Namespace}, service)

	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("[createService] Service not found, create it")
			err = r.Client.Create(ctx, newService)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		r.Log.Info("[createService] Service already present, update it")
		newService.SetResourceVersion(service.GetResourceVersion())
		return r.Client.Patch(ctx, service, client.MergeFrom(newService))
	}

	return nil
}

func (r *OranO2IMSReconciler) createIngress(ctx context.Context, orano2ims *oranv1alpha1.OranO2IMS) error {
	r.Log.Info("[createIngress]")
	// Build the Ingress object.
	ingressMeta := metav1.ObjectMeta{
		Name:      "api",
		Namespace: orano2ims.ObjectMeta.Namespace,
		Annotations: map[string]string{
			"route.openshift.io/termination": "reencrypt",
		},
	}

	ingressSpec := networkingv1.IngressSpec{
		Rules: []networkingv1.IngressRule{
			{
				Host: fmt.Sprintf("o2ims.%s", orano2ims.Spec.IngressHost),
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{
							{
								Path: "/o2ims-infrastructureInventory/v1/deploymentManagers",
								PathType: func() *networkingv1.PathType {
									pathType := networkingv1.PathTypePrefix
									return &pathType
								}(),
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "deployment-manager-server",
										Port: networkingv1.ServiceBackendPort{
											Name: "api",
										},
									},
								},
							},
							{
								Path: "/",
								PathType: func() *networkingv1.PathType {
									pathType := networkingv1.PathTypePrefix
									return &pathType
								}(),
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "metadata-server",
										Port: networkingv1.ServiceBackendPort{
											Name: "api",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	newIngress := &networkingv1.Ingress{
		ObjectMeta: ingressMeta,
		Spec:       ingressSpec,
	}

	// Set owner reference.
	if err := controllerutil.SetControllerReference(orano2ims, newIngress, r.Scheme); err != nil {
		return err
	}

	// Check if the Ingress already exists.
	ingress := &networkingv1.Ingress{}
	err := r.Get(ctx, types.NamespacedName{Name: "api", Namespace: orano2ims.ObjectMeta.Namespace}, ingress)

	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("[createIngress] Ingress not present, create it")
			err = r.Client.Create(ctx, newIngress)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		r.Log.Info("[createIngress] Ingress already present, update it")
		newIngress.SetResourceVersion(ingress.GetResourceVersion())
		return r.Client.Update(ctx, newIngress)
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

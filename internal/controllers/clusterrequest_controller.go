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

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	oranv1alpha1 "github.com/openshift-kni/oran-o2ims/api/v1alpha1"
	"github.com/openshift-kni/oran-o2ims/internal/controllers/utils"
)

// clusterRequestReconciler reconciles a ClusterRequest object
type ClusterRequestReconciler struct {
	client.Client
	Logger *slog.Logger
}

type clusterRequestReconcilerTask struct {
	logger *slog.Logger
	client client.Client
	object *oranv1alpha1.ClusterRequest
}

//+kubebuilder:rbac:groups=oran.openshift.io,resources=clusterrequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=oran.openshift.io,resources=clusterrequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=oran.openshift.io,resources=clusterrequests/finalizers,verbs=update

//+kubebuilder:rbac:groups=oran.openshift.io,resources=clustertemplates,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ClusterRequest object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *ClusterRequestReconciler) Reconcile(
	ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	_ = log.FromContext(ctx)

	// Fetch the object:
	object := &oranv1alpha1.ClusterRequest{}
	if err := r.Client.Get(ctx, req.NamespacedName, object); err != nil {
		if errors.IsNotFound(err) {
			err = nil
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}
		r.Logger.ErrorContext(
			ctx,
			"Unable to fetch Cluster Request",
			slog.String("error", err.Error()),
		)
	}

	r.Logger.InfoContext(ctx, "[Reconcile Cluster Request] "+object.Name)

	// Create and run the task:
	task := &clusterRequestReconcilerTask{
		logger: r.Logger,
		client: r.Client,
		object: object,
	}
	result, err = task.run(ctx)
	return
}

func (t *clusterRequestReconcilerTask) run(ctx context.Context) (nextReconcile ctrl.Result, err error) {
	// Set the default reconcile time to 5 minutes.
	nextReconcile = ctrl.Result{RequeueAfter: 5 * time.Minute}

	// ### JSON VALIDATION ###

	// Check if the clusterTemplateInput is in a JSON format; the schema itself is not of importance.
	err = utils.ValidateInputDataSchema(t.object.Spec.ClusterTemplateInput)

	// If there is an error, log it and set the reconciliation to 30 seconds.
	if err != nil {
		t.logger.ErrorContext(
			ctx,
			"clusterTemplateInput is not in a JSON format",
		)
		nextReconcile = ctrl.Result{RequeueAfter: 30 * time.Second}
	}

	// Update the ClusterRequest status.
	err = t.updateClusterTemplateInputValidationStatus(ctx, err)
	if err != nil {
		t.logger.ErrorContext(
			ctx,
			"Failed to update the ClusterTemplateInputValidation status for ClusterRequest",
			slog.String("name", t.object.Name),
		)
		nextReconcile = ctrl.Result{RequeueAfter: 30 * time.Second}
	}

	// ### VALIDATION AGAINST CLUSTER TEMPLATE REF ###

	// Check if the clusterTemplateInput matches the inputDataSchema of the ClusterTemplate
	// specified in spec.clusterTemplateRef.
	err = t.validateClusterTemplateInputMatchesClusterTemplate(ctx)
	if err != nil {
		t.logger.ErrorContext(
			ctx,
			err.Error(),
		)
		nextReconcile = ctrl.Result{RequeueAfter: 30 * time.Second}
	}

	// Update the ClusterRequest status.
	err = t.updateClusterTemplateMatchStatus(ctx, err)
	if err != nil {
		t.logger.ErrorContext(
			ctx,
			"Failed to update the ClusterTemplateValidation status for ClusterRequest",
			slog.String("name", t.object.Name),
		)
		nextReconcile = ctrl.Result{RequeueAfter: 30 * time.Second}
	}

	err = t.createClusterInstanceResources(ctx)

	return
}

// createClusterInstanceResources creates all the resources needed for the
// ClusterInstance object to perform a successful installation.
func (t *clusterRequestReconcilerTask) createClusterInstanceResources(
	ctx context.Context) error {

	// Create the clusterInstance namespace.
	err, clusterName := t.createClusterInstanceNamespace(ctx)
	if err != nil {
		return err
	}

	// Create the BMC secrets.
	err = t.createClusterInstanceBMCSecrets(ctx, clusterName)
	if err != nil {
		return err
	}

	return nil
}

// createClusterInstanceNamespace creates the namespace of the ClusterInstance
// where all the other resources needed for installation will exist.
func (t *clusterRequestReconcilerTask) createClusterInstanceNamespace(
	ctx context.Context) (error, string) {

	var inputData map[string]interface{}
	err := json.Unmarshal([]byte(t.object.Spec.ClusterTemplateInput), &inputData)
	if err != nil {
		return err, ""
	}

	// If we got to this point, we can assume that all the keys exist, including
	// clusterName
	clusterNameInterface, clusterNameExist := inputData["clusterName"]
	if !clusterNameExist {
		return fmt.Errorf(
			"\"clusterNameExist\" key expected to exist in ClusterTemplateInput of ClusterRequest %s, but it's missing",
			t.object.Name,
		), ""
	}

	clusterName := clusterNameInterface.(string)
	t.logger.InfoContext(
		ctx,
		"nodes: "+fmt.Sprintf("%v", clusterName),
	)

	// Create the namespace.
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterName,
		},
	}
	err = utils.CreateK8sCR(ctx, t.client, namespace, t.object, utils.UPDATE)
		if err != nil {
			return err, ""
		}

	return nil, clusterName
}

// createClusterInstanceBMCSecrets creates all the BMC secrets needed by the nodes included
// in the ClusterRequest.
func (t *clusterRequestReconcilerTask) createClusterInstanceBMCSecrets(
	ctx context.Context, clusterName string) error {

	var inputData map[string]interface{}
	err := json.Unmarshal([]byte(t.object.Spec.ClusterTemplateInput), &inputData)
	if err != nil {
		return err
	}

	// If we got to this point, we can assume that all the keys up to the BMC details
	// exists since ClusterInstance has nodes mandatory.
	nodesInterface, nodesExist := inputData["nodes"]
	if !nodesExist {
		return fmt.Errorf(
			"\"nodes\" key expected to exist in ClusterTemplateInput of ClusterRequest %s, but it's missing",
			t.object.Name,
		)
	}

	t.logger.InfoContext(
		ctx,
		"nodes: "+fmt.Sprintf("%v", nodesInterface),
	)

	nodes := nodesInterface.([]interface{})
	// Go through all the nodes.
	for _, nodeInterface := range nodes {
		node := nodeInterface.(map[string]interface{})
		
		username, password, secretName, err := 
		    utils.GetBMCDetailsForClusterInstance(node, t.object.Name)

		// Create the node's BMC secret.
		bmcSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: secretName,
				Namespace: clusterName,
			},
			Data: map[string][]byte{
				"username": []byte(username),
				"password": []byte(password),
			},
		}

		err = utils.CreateK8sCR(ctx, t.client, bmcSecret, t.object, utils.UPDATE)
		if err != nil {
			return err
		}
	}

	return nil
}

// validateClusterTemplateInput validates if the clusterTemplateInput matches the
// inputDataSchema of the ClusterTemplate
func (t *clusterRequestReconcilerTask) validateClusterTemplateInputMatchesClusterTemplate(
	ctx context.Context) error {

	// Check the clusterTemplateRef references an existing template in the same namespace
	// as the current clusterRequest.
	clusterTemplateRef := &oranv1alpha1.ClusterTemplate{}
	clusterTemplateRefExists, err := utils.DoesK8SResourceExist(
		ctx, t.client, t.object.Spec.ClusterTemplateRef, t.object.Namespace, clusterTemplateRef)

	// If there was an error in trying to get the ClusterTemplate, return it.
	if err != nil {
		t.logger.ErrorContext(
			ctx,
			"Error obtaining the ClusterTemplate referenced by ClusterTemplateRef",
			slog.String("clusterTemplateRef", t.object.Spec.ClusterTemplateRef),
		)
		return err
	}

	// If the referenced ClusterTemplate does not exist, log and return an appropriate error.
	if !clusterTemplateRefExists {
		err := fmt.Errorf(
			fmt.Sprintf(
				"The referenced ClusterTemplate (%s) does not exist in the %s namespace",
				t.object.Spec.ClusterTemplateRef, t.object.Namespace))

		t.logger.ErrorContext(
			ctx,
			err.Error())
		return err
	}

	// Check that the clusterTemplateInput matches the inputDataSchema from the ClusterTemplate.
	err = utils.ValidateJsonAgainstJsonSchema(
		clusterTemplateRef.Spec.InputDataSchema, t.object.Spec.ClusterTemplateInput)

	if err != nil {
		t.logger.ErrorContext(
			ctx,
			fmt.Sprintf(
				"The provided clusterTemplateInput does not match "+
					"the inputDataSchema from the ClusterTemplateRef (%s)",
				t.object.Spec.ClusterTemplateRef))

		return err
	}

	return nil
}

// updateClusterTemplateInputValidationStatus update the status of the ClusterTemplate object (CR).
func (t *clusterRequestReconcilerTask) updateClusterTemplateInputValidationStatus(
	ctx context.Context, inputError error) error {

	t.object.Status.ClusterTemplateInputValidation.InputIsValid = true
	t.object.Status.ClusterTemplateInputValidation.InputError = ""

	if inputError != nil {
		t.object.Status.ClusterTemplateInputValidation.InputIsValid = false
		t.object.Status.ClusterTemplateInputValidation.InputError = inputError.Error()
	}

	return utils.UpdateK8sCRStatus(ctx, t.client, t.object)
}

// updateClusterTemplateMatchStatus update the status of the ClusterTemplate object (CR).
func (t *clusterRequestReconcilerTask) updateClusterTemplateMatchStatus(
	ctx context.Context, inputError error) error {

	t.object.Status.ClusterTemplateInputValidation.InputMatchesTemplate = true
	t.object.Status.ClusterTemplateInputValidation.InputMatchesTemplateError = ""

	if inputError != nil {
		t.object.Status.ClusterTemplateInputValidation.InputMatchesTemplate = false
		t.object.Status.ClusterTemplateInputValidation.InputMatchesTemplateError = inputError.Error()
	}

	return utils.UpdateK8sCRStatus(ctx, t.client, t.object)
}

// findClusterRequestsForClusterTemplate maps the ClusterTemplates used by ClusterRequests
// to reconciling requests for those ClusterRequests.
func (r *ClusterRequestReconciler) findClusterRequestsForClusterTemplate(
	ctx context.Context, clusterTemplate client.Object) []reconcile.Request {

	// Empty array of reconciling requests.
	reqs := make([]reconcile.Request, 0)
	// Get all the clusterRequests.
	clusterRequests := &oranv1alpha1.ClusterRequestList{}
	err := r.Client.List(ctx, clusterRequests, client.InNamespace(clusterTemplate.GetNamespace()))
	if err != nil {
		return reqs
	}

	// Create reconciling requests only for the clusterRequests that are using the
	// current clusterTemplate.
	for _, clusterRequest := range clusterRequests.Items {
		if clusterRequest.Spec.ClusterTemplateRef == clusterTemplate.GetName() {
			reqs = append(
				reqs,
				reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: clusterRequest.Namespace,
						Name:      clusterRequest.Name,
					},
				},
			)
		}
	}

	return reqs
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("orano2ims-cluster-request").
		For(
			&oranv1alpha1.ClusterRequest{},
			// Watch for create and update event for ClusterRequest.
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(
			&oranv1alpha1.ClusterTemplate{},
			handler.EnqueueRequestsFromMapFunc(r.findClusterRequestsForClusterTemplate),
			builder.WithPredicates(predicate.Funcs{
				UpdateFunc: func(e event.UpdateEvent) bool {
					return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
				},
				CreateFunc:  func(ce event.CreateEvent) bool { return false },
				GenericFunc: func(ge event.GenericEvent) bool { return false },
				DeleteFunc:  func(de event.DeleteEvent) bool { return true },
			})).
		Complete(r)
}

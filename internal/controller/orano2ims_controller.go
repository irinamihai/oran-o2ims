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
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oranv1alpha1 "oran-o2ims/api/v1alpha1"
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
	var orano2ims oranv1alpha1.OranO2IMS
	if err := r.Get(ctx, req.NamespacedName, &orano2ims); err != nil {
		r.Log.Error(err, "Unable to fetch Oran")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	r.Log.Info(">>> Got orano2ims", "lala", orano2ims.Spec.CloudId)
	nextReconcile = ctrl.Result{RequeueAfter: 5 * time.Second}

	return
}

// SetupWithManager sets up the controller with the Manager.
func (r *OranO2IMSReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&oranv1alpha1.OranO2IMS{}).
		Complete(r)
}

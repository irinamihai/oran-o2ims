package utils

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var oranUtilsLog = ctrl.Log.WithName("oranUtilsLog")


func doesK8SResourceExist(ctx context.Context, c client.Client, Name string, Namespace string, obj client.Object) (resourceExists bool, err error) {

	err = c.Get(ctx, types.NamespacedName{Name: Name, Namespace: Namespace}, obj)

	if err != nil {
	    if errors.IsNotFound(err) {
			oranUtilsLog.Info("[doesK8SResourceExist] Resource not found, create it")
			return false, nil
		} else {
			return false, err
		}
	} else {
		oranUtilsLog.Info("[doesK8SResourceExist] Resource already present, return")
		return true, nil
	}
}
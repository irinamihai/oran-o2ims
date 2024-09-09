package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	oranv1alpha1 "github.com/openshift-kni/oran-o2ims/api/v1alpha1"
	"github.com/openshift-kni/oran-o2ims/internal/controllers/utils"
	siteconfig "github.com/stolostron/siteconfig/api/v1alpha1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

// handleClusterPolicyConfiguration updates the ClusterRequest status to reflect the status
// of the policies that match the managed cluster created through the ClusterRequest.
func (t *clusterRequestReconcilerTask) handleClusterPolicyConfiguration(ctx context.Context) (
	requeue bool, err error) {
	if t.object.Status.ClusterInstanceRef == nil {
		return false, fmt.Errorf("status.clusterInstanceRef is empty")
	}

	clusterIsReadyForPolicyConfig, err := utils.ClusterIsReadyForPolicyConfig(
		ctx, t.client, t.object.Status.ClusterInstanceRef.Name,
	)
	if err != nil {
		return false, fmt.Errorf(
			"error determining if the cluster is ready for policy configuration: %w", err)
	}

	if !clusterIsReadyForPolicyConfig {
		t.logger.InfoContext(
			ctx,
			fmt.Sprintf(
				"Cluster %s (%s) is not ready for policy configuration",
				t.object.Status.ClusterInstanceRef.Name,
				t.object.Status.ClusterInstanceRef.Name,
			),
		)
		return false, nil
	}

	// Get all the child policies in the namespace of the managed cluster created through
	// the ClusterRequest.
	policies := &policiesv1.PolicyList{}
	listOpts := []client.ListOption{
		client.HasLabels{utils.ChildPolicyRootPolicyLabel},
		client.InNamespace(t.object.Status.ClusterInstanceRef.Name),
	}

	err = t.client.List(ctx, policies, listOpts...)
	if err != nil {
		return false, fmt.Errorf("failed to list Policies: %w", err)
	}

	allPoliciesCompliant := true
	nonCompliantPolicyInEnforce := false
	var targetPolicies []oranv1alpha1.PolicyDetails
	// Go through all the policies and get those that are matched with the managed cluster created
	// by the current cluster request.
	for _, policy := range policies.Items {
		if policy.Status.ComplianceState != policiesv1.Compliant {
			allPoliciesCompliant = false
			if strings.EqualFold(string(policy.Spec.RemediationAction), string(policiesv1.Enforce)) {
				nonCompliantPolicyInEnforce = true
			}
		}
		// Child policy name = parent_policy_namespace.parent_policy_name
		policyNameArr := strings.Split(policy.Name, ".")
		targetPolicy := &oranv1alpha1.PolicyDetails{
			Compliant:         string(policy.Status.ComplianceState),
			PolicyName:        policyNameArr[1],
			PolicyNamespace:   policyNameArr[0],
			RemediationAction: string(policy.Spec.RemediationAction),
		}
		targetPolicies = append(targetPolicies, *targetPolicy)
	}
	err = t.updateConfigurationAppliedStatus(
		ctx, targetPolicies, allPoliciesCompliant, nonCompliantPolicyInEnforce)
	if err != nil {
		return false, err
	}

	// If there are policies that are not Compliant, we need to requeue and see if they
	// time out or complete.
	return nonCompliantPolicyInEnforce, nil
}

// hasPolicyConfigurationTimedOut determines if the policy configuration for the
// ClusterRequest has timed out.
func (t *clusterRequestReconcilerTask) hasPolicyConfigurationTimedOut(ctx context.Context) bool {
	policyTimedOut := false
	// Get the ConfigurationApplied condition.
	configurationAppliedCondition := meta.FindStatusCondition(
		t.object.Status.Conditions,
		string(utils.CRconditionTypes.ConfigurationApplied))

	// If the condition does not exist, set the non compliant timestamp since we
	// get here just for policies that have a status different from Compliant.
	if configurationAppliedCondition == nil {
		t.object.Status.ClusterInstanceRef.NonCompliantAt = metav1.Now()
		return policyTimedOut
	}

	// If the current status of the Condition is false.
	if configurationAppliedCondition.Status == metav1.ConditionFalse {
		switch configurationAppliedCondition.Reason {
		case string(utils.CRconditionReasons.InProgress):
			// Check if the configuration application has timed out.
			if time.Since(t.object.Status.ClusterInstanceRef.NonCompliantAt.Time) > time.Duration(t.object.Spec.Timeout.Configuration) {
				policyTimedOut = true
			}
		case string(utils.CRconditionReasons.TimedOut):
			policyTimedOut = true
		case string(utils.CRconditionReasons.Missing):
			t.object.Status.ClusterInstanceRef.NonCompliantAt = metav1.Now()
		case string(utils.CRconditionReasons.OutOfDate):
			t.object.Status.ClusterInstanceRef.NonCompliantAt = metav1.Now()
		default:
			t.logger.InfoContext(ctx,
				fmt.Sprintf("Unexpected Reason for condition type %s",
					utils.CRconditionTypes.ConfigurationApplied,
				),
			)
		}
	} else if configurationAppliedCondition.Reason == string(utils.CRconditionReasons.Completed) {
		t.object.Status.ClusterInstanceRef.NonCompliantAt = metav1.Now()
	}

	return policyTimedOut
}

// updateConfigurationAppliedStatus updates the ClusterRequest ConfigurationApplied condition
// based on the state of the policies matched with the managed cluster.
func (t *clusterRequestReconcilerTask) updateConfigurationAppliedStatus(
	ctx context.Context, targetPolicies []oranv1alpha1.PolicyDetails, allPoliciesCompliant bool,
	nonCompliantPolicyInEnforce bool) error {
	if len(targetPolicies) == 0 {
		t.object.Status.ClusterInstanceRef.NonCompliantAt = metav1.Time{}
		utils.SetStatusCondition(&t.object.Status.Conditions,
			utils.CRconditionTypes.ConfigurationApplied,
			utils.CRconditionReasons.Missing,
			metav1.ConditionFalse,
			"No configuration present",
		)
	} else {
		// Update the ConfigurationApplied condition.
		if allPoliciesCompliant {
			t.object.Status.ClusterInstanceRef.NonCompliantAt = metav1.Time{}
			utils.SetStatusCondition(&t.object.Status.Conditions,
				utils.CRconditionTypes.ConfigurationApplied,
				utils.CRconditionReasons.Completed,
				metav1.ConditionTrue,
				"The configuration is up to date",
			)
		} else {
			if nonCompliantPolicyInEnforce {
				policyTimedOut := t.hasPolicyConfigurationTimedOut(ctx)

				message := "The configuration is still being applied"
				reason := utils.CRconditionReasons.InProgress
				if policyTimedOut {
					message += ", but it timed out"
					reason = utils.CRconditionReasons.TimedOut
				}
				utils.SetStatusCondition(&t.object.Status.Conditions,
					utils.CRconditionTypes.ConfigurationApplied,
					reason,
					metav1.ConditionFalse,
					message,
				)
			} else {
				// No timeout is reported if all policies are in inform, just out of date.
				t.object.Status.ClusterInstanceRef.NonCompliantAt = metav1.Time{}
				utils.SetStatusCondition(&t.object.Status.Conditions,
					utils.CRconditionTypes.ConfigurationApplied,
					utils.CRconditionReasons.OutOfDate,
					metav1.ConditionFalse,
					"The configuration is out of date",
				)
			}
		}
	}

	t.object.Status.Policies = targetPolicies
	// Update the current policy status.
	if updateErr := utils.UpdateK8sCRStatus(ctx, t.client, t.object); updateErr != nil {
		return fmt.Errorf("failed to update status for ClusterRequest %s: %w", t.object.Name, updateErr)
	}

	return nil
}

// findPoliciesForClusterRequests creates reconciliation requests for the ClusterRequests
// whose associated ManagedClusters have matched policies Updated or Deleted.
func findPoliciesForClusterRequests[T deleteOrUpdateEvent](
	ctx context.Context, c client.Client, e T, q workqueue.RateLimitingInterface) {

	policy := &policiesv1.Policy{}
	switch evt := any(e).(type) {
	case event.UpdateEvent:
		policy = evt.ObjectOld.(*policiesv1.Policy)
	case event.DeleteEvent:
		policy = evt.Object.(*policiesv1.Policy)
	default:
		// Only Update and Delete events are supported
		return
	}

	// Get the ClusterInstance.
	clusterInstance := &siteconfig.ClusterInstance{}
	err := c.Get(ctx, types.NamespacedName{
		Namespace: policy.Namespace,
		Name:      policy.Namespace,
	}, clusterInstance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Return as the ManagedCluster for this Namespace is not deployed/managed by ClusterInstance.
			return
		}
		return
	}

	clusterRequest, okCR := clusterInstance.GetLabels()[clusterRequestNameLabel]
	clusterRequestNs, okCRNs := clusterInstance.GetLabels()[clusterRequestNamespaceLabel]
	if okCR && okCRNs {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      clusterRequest,
			Namespace: clusterRequestNs,
		}})
	}
}

// handlePolicyEvent handled Updates and Deleted events.
func (r *ClusterRequestReconciler) handlePolicyEventDelete(
	ctx context.Context, e event.DeleteEvent, q workqueue.RateLimitingInterface) {

	// Call the generic function for determining the corresponding ClusterRequest.
	findPoliciesForClusterRequests(ctx, r.Client, e, q)
}

// handlePolicyEvent handled Updates and Deleted events.
func (r *ClusterRequestReconciler) handlePolicyEventUpdate(
	ctx context.Context, e event.UpdateEvent, q workqueue.RateLimitingInterface) {

	// Call the generic function.
	findPoliciesForClusterRequests(ctx, r.Client, e, q)
}

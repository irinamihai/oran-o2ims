package controllers

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	siteconfig "github.com/stolostron/siteconfig/api/v1alpha1"

	oranv1alpha1 "github.com/openshift-kni/oran-o2ims/api/v1alpha1"
	"github.com/openshift-kni/oran-o2ims/internal/controllers/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("policyManagement", func() {
	var (
		ctx          context.Context
		c            client.Client
		CRReconciler *ClusterRequestReconciler
		CRTask       *clusterRequestReconcilerTask
		CTReconciler *ClusterTemplateReconciler
		ctName       = "clustertemplate-a-v1"
		ctNamespace  = "clustertemplate-a-v4-16"
		ciDefaultsCm = "clusterinstance-defaults-v1"
		ptDefaultsCm = "policytemplate-defaults-v1"
		hwTemplateCm = "hwTemplate-v1"
		updateEvent  *event.UpdateEvent
	)

	BeforeEach(func() {
		// Define the needed resources.
		crs := []client.Object{
			// Cluster Template Namespace.
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: ctNamespace,
				},
			},
			// Cluster Template.
			&oranv1alpha1.ClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ctName,
					Namespace: ctNamespace,
				},
				Spec: oranv1alpha1.ClusterTemplateSpec{
					Templates: oranv1alpha1.Templates{
						ClusterInstanceDefaults: ciDefaultsCm,
						PolicyTemplateDefaults:  ptDefaultsCm,
						HwTemplate:              hwTemplateCm,
					},
					InputDataSchema: oranv1alpha1.InputDataSchema{
						// APIserver has enforced the validation for this field who holds
						// the arbirary JSON data
						ClusterInstanceSchema: runtime.RawExtension{
							Raw: []byte(testClusterTemplateSchema),
						},
						PolicyTemplateSchema: runtime.RawExtension{
							Raw: []byte(testPolicyTemplateSchema),
						},
					},
				},
			},
			// ConfigMaps.
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ciDefaultsCm,
					Namespace: ctNamespace,
				},
				Data: map[string]string{
					utils.ClusterInstanceTemplateDefaultsConfigmapKey: `
clusterImageSetNameRef: "4.15"
pullSecretRef:
  name: "pull-secret"
templateRefs:
- name: "ai-cluster-templates-v1"
  namespace: "siteconfig-operator"
nodes:
- hostname: "node1"
  templateRefs:
  - name: "ai-node-templates-v1"
    namespace: "siteconfig-operator"
`,
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foobar1",
					Namespace: ctNamespace,
				},
				Data: map[string]string{},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ptDefaultsCm,
					Namespace: ctNamespace,
				},
				Data: map[string]string{
					utils.PolicyTemplateDefaultsConfigmapKey: `
cpu-isolated: "2-31"
cpu-reserved: "0-1"
defaultHugepagesSize: "1G"`,
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hwTemplateCm,
					Namespace: utils.ORANO2IMSNamespace,
				},
				Data: map[string]string{
					"hwMgrId": "hwmgr",
					utils.HwTemplateNodePool: `
- name: master
  hwProfile: profile-spr-single-processor-64G
- name: worker
  hwProfile: profile-spr-dual-processor-128G`,
				},
			},
			// Pull secret.
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pull-secret",
					Namespace: ctNamespace,
				},
			},
			// Cluster Requests.
			&oranv1alpha1.ClusterRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "cluster-1",
					Namespace:  ctNamespace,
					Finalizers: []string{clusterRequestFinalizer},
				},
				Spec: oranv1alpha1.ClusterRequestSpec{
					ClusterTemplateRef: ctName,
					ClusterTemplateInput: oranv1alpha1.ClusterTemplateInput{
						ClusterInstanceInput: runtime.RawExtension{
							Raw: []byte(testClusterTemplateInput),
						},
						PolicyTemplateInput: runtime.RawExtension{
							Raw: []byte(testPolicyTemplateInput),
						},
					},
					Timeout: oranv1alpha1.Timeout{
						Configuration: 1,
					},
				},
				Status: oranv1alpha1.ClusterRequestStatus{
					// Fake the hw provision status
					Conditions: []metav1.Condition{
						{
							Type:   string(utils.CRconditionTypes.HardwareProvisioned),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			// Managed clusters
			&clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-1",
				},
				Spec: clusterv1.ManagedClusterSpec{
					HubAcceptsClient: true,
				},
			},
		}

		c = getFakeClientFromObjects(crs...)
		// Reconcile the ClusterTemplate.
		CTReconciler = &ClusterTemplateReconciler{
			Client: c,
			Logger: logger,
		}

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      ctName,
				Namespace: ctNamespace,
			},
		}

		_, err := CTReconciler.Reconcile(ctx, req)
		Expect(err).ToNot(HaveOccurred())

		CRReconciler = &ClusterRequestReconciler{
			Client: c,
			Logger: logger,
		}

		// Update the managedCluster cluster-1 to be available, joined and accepted.
		managedCluster1 := &clusterv1.ManagedCluster{}
		managedClusterExists, err := utils.DoesK8SResourceExist(
			ctx, c, "cluster-1", "", managedCluster1)
		Expect(err).ToNot(HaveOccurred())
		Expect(managedClusterExists).To(BeTrue())
		utils.SetStatusCondition(&managedCluster1.Status.Conditions,
			utils.ConditionType(clusterv1.ManagedClusterConditionAvailable),
			"ManagedClusterAvailable",
			metav1.ConditionTrue,
			"Managed cluster is available",
		)
		utils.SetStatusCondition(&managedCluster1.Status.Conditions,
			utils.ConditionType(clusterv1.ManagedClusterConditionHubAccepted),
			"HubClusterAdminAccepted",
			metav1.ConditionTrue,
			"Accepted by hub cluster admin",
		)
		utils.SetStatusCondition(&managedCluster1.Status.Conditions,
			utils.ConditionType(clusterv1.ManagedClusterConditionJoined),
			"ManagedClusterJoined",
			metav1.ConditionTrue,
			"Managed cluster joined",
		)
		err = CRReconciler.Client.Status().Update(context.TODO(), managedCluster1)
		Expect(err).ToNot(HaveOccurred())
	})

	It("Moves from TimedOut to Completed if all the policies are compliant", func() {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "cluster-1",
				Namespace: ctNamespace,
			},
		}

		result, err := CRReconciler.Reconcile(ctx, req)
		Expect(err).ToNot(HaveOccurred())
		// Expect to not requeue on valid cluster request.
		Expect(result.Requeue).To(BeFalse())

		clusterRequest := &oranv1alpha1.ClusterRequest{}

		// Create the ClusterRequest reconciliation task.
		err = CRReconciler.Client.Get(
			context.TODO(),
			types.NamespacedName{
				Name:      "cluster-1",
				Namespace: "clustertemplate-a-v4-16",
			},
			clusterRequest)
		Expect(err).ToNot(HaveOccurred())

		CRTask = &clusterRequestReconcilerTask{
			logger: CRReconciler.Logger,
			client: CRReconciler.Client,
			object: clusterRequest, // cluster-1 request
		}

		// Update the ClusterRequest ConfigurationApplied condition to TimedOut.
		utils.SetStatusCondition(&CRTask.object.Status.Conditions,
			utils.CRconditionTypes.ConfigurationApplied,
			utils.CRconditionReasons.TimedOut,
			metav1.ConditionFalse,
			"The configuration is still being applied, but it timed out",
		)
		Expect(c.Status().Update(ctx, CRTask.object)).To(Succeed())

		// Create the policies, all Compliant, one in inform and one in enforce.
		newPolicies := []client.Object{
			&policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ztp-clustertemplate-a-v4-16.v1-subscriptions-policy",
					Namespace: "cluster-1",
					Labels: map[string]string{
						utils.ChildPolicyRootPolicyLabel:       "ztp-clustertemplate-a-v4-16.v1-subscriptions-policy",
						utils.ChildPolicyClusterNameLabel:      "cluster-1",
						utils.ChildPolicyClusterNamespaceLabel: "cluster-1",
					},
				},
				Spec: policiesv1.PolicySpec{
					RemediationAction: "enforce",
				},
				Status: policiesv1.PolicyStatus{
					ComplianceState: "Compliant",
				},
			},
			&policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ztp-clustertemplate-a-v4-16.v1-sriov-configuration-policy",
					Namespace: "cluster-1",
					Labels: map[string]string{
						utils.ChildPolicyRootPolicyLabel:       "ztp-clustertemplate-a-v4-16.v1-sriov-configuration-policy",
						utils.ChildPolicyClusterNameLabel:      "cluster-1",
						utils.ChildPolicyClusterNamespaceLabel: "cluster-1",
					},
				},
				Spec: policiesv1.PolicySpec{
					RemediationAction: "inform",
				},
				Status: policiesv1.PolicyStatus{
					ComplianceState: "Compliant",
				},
			},
		}
		for _, newPolicy := range newPolicies {
			Expect(c.Create(ctx, newPolicy)).To(Succeed())
		}

		// Call the handleClusterPolicyConfiguration function.
		requeue, err := CRTask.handleClusterPolicyConfiguration(context.Background())
		Expect(requeue).To(BeFalse()) // there are no NonCompliant policies
		Expect(err).ToNot(HaveOccurred())
		Expect(CRTask.object.Status.ClusterInstanceRef.NonCompliantAt).To(BeZero())
		Expect(CRTask.object.Status.Policies).To(ConsistOf(
			[]oranv1alpha1.PolicyDetails{
				{
					Compliant:         "Compliant",
					PolicyName:        "v1-sriov-configuration-policy",
					PolicyNamespace:   "ztp-clustertemplate-a-v4-16",
					RemediationAction: "inform",
				},
				{
					Compliant:         "Compliant",
					PolicyName:        "v1-subscriptions-policy",
					PolicyNamespace:   "ztp-clustertemplate-a-v4-16",
					RemediationAction: "enforce",
				},
			},
		))

		// Check the status conditions.
		conditions := CRTask.object.Status.Conditions
		configAppliedCond := meta.FindStatusCondition(
			conditions, string(utils.CRconditionTypes.ConfigurationApplied))
		Expect(configAppliedCond).ToNot(BeNil())
		Expect(configAppliedCond.Type).To(Equal(string(utils.CRconditionTypes.ConfigurationApplied)))
		Expect(configAppliedCond.Status).To(Equal(metav1.ConditionTrue))
		Expect(configAppliedCond.Reason).To(Equal(string(utils.CRconditionReasons.Completed)))
		Expect(configAppliedCond.Message).To(Equal("The configuration is up to date"))
	})

	It("Clears the NonCompliantAt timestamp and timeout when policies are switched to inform", func() {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "cluster-1",
				Namespace: ctNamespace,
			},
		}

		result, err := CRReconciler.Reconcile(ctx, req)
		Expect(err).ToNot(HaveOccurred())
		// Expect to not requeue on valid cluster request.
		Expect(result.Requeue).To(BeFalse())

		clusterRequest := &oranv1alpha1.ClusterRequest{}

		// Create the ClusterRequest reconciliation task.
		err = CRReconciler.Client.Get(
			context.TODO(),
			types.NamespacedName{
				Name:      "cluster-1",
				Namespace: "clustertemplate-a-v4-16",
			},
			clusterRequest)
		Expect(err).ToNot(HaveOccurred())

		CRTask = &clusterRequestReconcilerTask{
			logger: CRReconciler.Logger,
			client: CRReconciler.Client,
			object: clusterRequest, // cluster-1 request
		}

		// Create inform policies, one Compliant and one NonCompliant.
		newPolicies := []client.Object{
			&policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ztp-clustertemplate-a-v4-16.v1-subscriptions-policy",
					Namespace: "cluster-1",
					Labels: map[string]string{
						utils.ChildPolicyRootPolicyLabel:       "ztp-clustertemplate-a-v4-16.v1-subscriptions-policy",
						utils.ChildPolicyClusterNameLabel:      "cluster-1",
						utils.ChildPolicyClusterNamespaceLabel: "cluster-1",
					},
				},
				Spec: policiesv1.PolicySpec{
					RemediationAction: "enforce",
				},
				Status: policiesv1.PolicyStatus{
					ComplianceState: "NonCompliant",
				},
			},
			&policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ztp-clustertemplate-a-v4-16.v1-sriov-configuration-policy",
					Namespace: "cluster-1",
					Labels: map[string]string{
						utils.ChildPolicyRootPolicyLabel:       "ztp-clustertemplate-a-v4-16.v1-sriov-configuration-policy",
						utils.ChildPolicyClusterNameLabel:      "cluster-1",
						utils.ChildPolicyClusterNamespaceLabel: "cluster-1",
					},
				},
				Spec: policiesv1.PolicySpec{
					RemediationAction: "inform",
				},
				Status: policiesv1.PolicyStatus{
					ComplianceState: "Compliant",
				},
			},
		}
		for _, newPolicy := range newPolicies {
			Expect(c.Create(ctx, newPolicy)).To(Succeed())
		}

		// Call the handleClusterPolicyConfiguration function.
		requeue, err := CRTask.handleClusterPolicyConfiguration(context.Background())
		Expect(requeue).To(BeTrue()) // there are NonCompliant policies in enforce
		Expect(err).ToNot(HaveOccurred())
		Expect(CRTask.object.Status.ClusterInstanceRef.NonCompliantAt).ToNot(BeZero())
		Expect(CRTask.object.Status.Policies).To(ConsistOf(
			[]oranv1alpha1.PolicyDetails{
				{
					Compliant:         "Compliant",
					PolicyName:        "v1-sriov-configuration-policy",
					PolicyNamespace:   "ztp-clustertemplate-a-v4-16",
					RemediationAction: "inform",
				},
				{
					Compliant:         "NonCompliant",
					PolicyName:        "v1-subscriptions-policy",
					PolicyNamespace:   "ztp-clustertemplate-a-v4-16",
					RemediationAction: "enforce",
				},
			},
		))

		// Check the status conditions.
		conditions := CRTask.object.Status.Conditions
		configAppliedCond := meta.FindStatusCondition(
			conditions, string(utils.CRconditionTypes.ConfigurationApplied))
		Expect(configAppliedCond).ToNot(BeNil())
		Expect(configAppliedCond.Type).To(Equal(string(utils.CRconditionTypes.ConfigurationApplied)))
		Expect(configAppliedCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(configAppliedCond.Reason).To(Equal(string(utils.CRconditionReasons.InProgress)))
		Expect(configAppliedCond.Message).To(Equal("The configuration is still being applied"))

		// Take 2 minutes to the NonCompliantAt timestamp to mock timeout.
		CRTask.object.Status.ClusterInstanceRef.NonCompliantAt.Time =
			CRTask.object.Status.ClusterInstanceRef.NonCompliantAt.Add(-2 * time.Minute)
		Expect(c.Status().Update(ctx, CRTask.object)).To(Succeed())

		// Call the handleClusterPolicyConfiguration function.
		requeue, err = CRTask.handleClusterPolicyConfiguration(context.Background())
		Expect(requeue).To(BeTrue()) // there are NonCompliant policies in enforce
		Expect(err).ToNot(HaveOccurred())

		// Check the status conditions.
		conditions = CRTask.object.Status.Conditions
		configAppliedCond = meta.FindStatusCondition(
			conditions, string(utils.CRconditionTypes.ConfigurationApplied))
		Expect(configAppliedCond).ToNot(BeNil())
		Expect(configAppliedCond.Type).To(Equal(string(utils.CRconditionTypes.ConfigurationApplied)))
		Expect(configAppliedCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(configAppliedCond.Reason).To(Equal(string(utils.CRconditionReasons.TimedOut)))
		Expect(configAppliedCond.Message).To(
			Equal("The configuration is still being applied, but it timed out"))

		// Check that the NonCompliantAt and timeout are cleared if the policies are in inform.
		// Inform the NonCompliant policy.
		policy := &policiesv1.Policy{}
		policyExists, err := utils.DoesK8SResourceExist(
			ctx, c, "ztp-clustertemplate-a-v4-16.v1-subscriptions-policy", "cluster-1", policy)
		Expect(err).ToNot(HaveOccurred())
		Expect(policyExists).To(BeTrue())
		policy.Spec.RemediationAction = policiesv1.Inform
		Expect(c.Update(ctx, policy)).To(Succeed())
		requeue, err = CRTask.handleClusterPolicyConfiguration(context.Background())
		Expect(requeue).To(BeFalse()) // all policies are in inform
		Expect(err).ToNot(HaveOccurred())
		// Check that the NonCompliantAt is zero.
		Expect(CRTask.object.Status.ClusterInstanceRef.NonCompliantAt).To(BeZero())

		// Check the status conditions.
		conditions = CRTask.object.Status.Conditions
		configAppliedCond = meta.FindStatusCondition(
			conditions, string(utils.CRconditionTypes.ConfigurationApplied))
		Expect(configAppliedCond).ToNot(BeNil())
		Expect(configAppliedCond.Type).To(Equal(string(utils.CRconditionTypes.ConfigurationApplied)))
		Expect(configAppliedCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(configAppliedCond.Reason).To(Equal(string(utils.CRconditionReasons.OutOfDate)))
		Expect(configAppliedCond.Message).To(
			Equal("The configuration is out of date"))
	})

	It("Sets the NonCompliantAt timestamp and times out", func() {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "cluster-1",
				Namespace: ctNamespace,
			},
		}

		result, err := CRReconciler.Reconcile(ctx, req)
		Expect(err).ToNot(HaveOccurred())
		// Expect to not requeue on valid cluster request.
		Expect(result.Requeue).To(BeFalse())

		clusterRequest := &oranv1alpha1.ClusterRequest{}

		// Create the ClusterRequest reconciliation task.
		err = CRReconciler.Client.Get(
			context.TODO(),
			types.NamespacedName{
				Name:      "cluster-1",
				Namespace: "clustertemplate-a-v4-16",
			},
			clusterRequest)
		Expect(err).ToNot(HaveOccurred())

		CRTask = &clusterRequestReconcilerTask{
			logger: CRReconciler.Logger,
			client: CRReconciler.Client,
			object: clusterRequest, // cluster-1 request
		}

		// Call the handleClusterPolicyConfiguration function.
		requeue, err := CRTask.handleClusterPolicyConfiguration(context.Background())
		Expect(requeue).To(BeFalse())
		Expect(err).ToNot(HaveOccurred())
		Expect(CRTask.object.Status.Policies).To(BeEmpty())
		Expect(CRTask.object.Status.ClusterInstanceRef.NonCompliantAt).To(BeZero())

		// Create inform policies, one Compliant and one NonCompliant.
		newPolicies := []client.Object{
			&policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ztp-clustertemplate-a-v4-16.v1-subscriptions-policy",
					Namespace: "cluster-1",
					Labels: map[string]string{
						utils.ChildPolicyRootPolicyLabel:       "ztp-clustertemplate-a-v4-16.v1-subscriptions-policy",
						utils.ChildPolicyClusterNameLabel:      "cluster-1",
						utils.ChildPolicyClusterNamespaceLabel: "cluster-1",
					},
				},
				Spec: policiesv1.PolicySpec{
					RemediationAction: "inform",
				},
				Status: policiesv1.PolicyStatus{
					ComplianceState: "NonCompliant",
				},
			},
			&policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ztp-clustertemplate-a-v4-16.v1-sriov-configuration-policy",
					Namespace: "cluster-1",
					Labels: map[string]string{
						utils.ChildPolicyRootPolicyLabel:       "ztp-clustertemplate-a-v4-16.v1-sriov-configuration-policy",
						utils.ChildPolicyClusterNameLabel:      "cluster-1",
						utils.ChildPolicyClusterNamespaceLabel: "cluster-1",
					},
				},
				Spec: policiesv1.PolicySpec{
					RemediationAction: "inform",
				},
				Status: policiesv1.PolicyStatus{
					ComplianceState: "Compliant",
				},
			},
		}
		for _, newPolicy := range newPolicies {
			Expect(c.Create(ctx, newPolicy)).To(Succeed())
		}

		// Call the handleClusterPolicyConfiguration function.
		requeue, err = CRTask.handleClusterPolicyConfiguration(context.Background())
		Expect(requeue).To(BeFalse())
		Expect(err).ToNot(HaveOccurred())
		// NonCompliantAt should still be used since we don't consider inform policies in the timeout.
		Expect(CRTask.object.Status.ClusterInstanceRef.NonCompliantAt).To(BeZero())
		Expect(CRTask.object.Status.Policies).To(ConsistOf(
			[]oranv1alpha1.PolicyDetails{
				{
					Compliant:         "Compliant",
					PolicyName:        "v1-sriov-configuration-policy",
					PolicyNamespace:   "ztp-clustertemplate-a-v4-16",
					RemediationAction: "inform",
				},
				{
					Compliant:         "NonCompliant",
					PolicyName:        "v1-subscriptions-policy",
					PolicyNamespace:   "ztp-clustertemplate-a-v4-16",
					RemediationAction: "inform",
				},
			},
		))

		// Check the status conditions.
		conditions := CRTask.object.Status.Conditions
		configAppliedCond := meta.FindStatusCondition(
			conditions, string(utils.CRconditionTypes.ConfigurationApplied))
		Expect(configAppliedCond).ToNot(BeNil())
		Expect(configAppliedCond.Type).To(Equal(string(utils.CRconditionTypes.ConfigurationApplied)))
		Expect(configAppliedCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(configAppliedCond.Reason).To(Equal(string(utils.CRconditionReasons.OutOfDate)))
		Expect(configAppliedCond.Message).To(Equal("The configuration is out of date"))

		// Enforce the NonCompliant policy.
		policy := &policiesv1.Policy{}
		policyExists, err := utils.DoesK8SResourceExist(
			ctx, c, "ztp-clustertemplate-a-v4-16.v1-subscriptions-policy", "cluster-1", policy)
		Expect(err).ToNot(HaveOccurred())
		Expect(policyExists).To(BeTrue())
		policy.Spec.RemediationAction = policiesv1.Enforce
		Expect(c.Update(ctx, policy)).To(Succeed())

		policyExists, err = utils.DoesK8SResourceExist(
			ctx, c, "ztp-clustertemplate-a-v4-16.v1-subscriptions-policy", "cluster-1", policy)
		Expect(err).ToNot(HaveOccurred())
		Expect(policyExists).To(BeTrue())

		// Call the handleClusterPolicyConfiguration function.
		requeue, err = CRTask.handleClusterPolicyConfiguration(context.Background())
		Expect(requeue).To(BeTrue()) // there are NonCompliant policies in enforce
		Expect(err).ToNot(HaveOccurred())
		Expect(CRTask.object.Status.ClusterInstanceRef.NonCompliantAt).ToNot(BeZero())
		Expect(CRTask.object.Status.Policies).To(ConsistOf(
			[]oranv1alpha1.PolicyDetails{
				{
					Compliant:         "Compliant",
					PolicyName:        "v1-sriov-configuration-policy",
					PolicyNamespace:   "ztp-clustertemplate-a-v4-16",
					RemediationAction: "inform",
				},
				{
					Compliant:         "NonCompliant",
					PolicyName:        "v1-subscriptions-policy",
					PolicyNamespace:   "ztp-clustertemplate-a-v4-16",
					RemediationAction: "Enforce",
				},
			},
		))

		// Check the status conditions.
		conditions = CRTask.object.Status.Conditions
		configAppliedCond = meta.FindStatusCondition(
			conditions, string(utils.CRconditionTypes.ConfigurationApplied))
		Expect(configAppliedCond).ToNot(BeNil())
		Expect(configAppliedCond.Type).To(Equal(string(utils.CRconditionTypes.ConfigurationApplied)))
		Expect(configAppliedCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(configAppliedCond.Reason).To(Equal(string(utils.CRconditionReasons.InProgress)))
		Expect(configAppliedCond.Message).To(Equal("The configuration is still being applied"))

		// Take 2 minutes to the NonCompliantAt timestamp to mock timeout.
		CRTask.object.Status.ClusterInstanceRef.NonCompliantAt.Time =
			CRTask.object.Status.ClusterInstanceRef.NonCompliantAt.Add(-2 * time.Minute)
		Expect(c.Status().Update(ctx, CRTask.object)).To(Succeed())

		// Call the handleClusterPolicyConfiguration function.
		requeue, err = CRTask.handleClusterPolicyConfiguration(context.Background())
		Expect(requeue).To(BeTrue()) // there are NonCompliant policies in enforce
		Expect(err).ToNot(HaveOccurred())

		// Check the status conditions.
		conditions = CRTask.object.Status.Conditions
		configAppliedCond = meta.FindStatusCondition(
			conditions, string(utils.CRconditionTypes.ConfigurationApplied))
		Expect(configAppliedCond).ToNot(BeNil())
		Expect(configAppliedCond.Type).To(Equal(string(utils.CRconditionTypes.ConfigurationApplied)))
		Expect(configAppliedCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(configAppliedCond.Reason).To(Equal(string(utils.CRconditionReasons.TimedOut)))
		Expect(configAppliedCond.Message).To(
			Equal("The configuration is still being applied, but it timed out"))

		// Check that another handleClusterPolicyConfiguration call doesn't change the status if
		// the policies are the same.
		requeue, err = CRTask.handleClusterPolicyConfiguration(context.Background())
		Expect(requeue).To(BeTrue()) // there are NonCompliant policies in enforce
		Expect(err).ToNot(HaveOccurred())

		// Check the status conditions.
		conditions = CRTask.object.Status.Conditions
		configAppliedCond = meta.FindStatusCondition(
			conditions, string(utils.CRconditionTypes.ConfigurationApplied))
		Expect(configAppliedCond).ToNot(BeNil())
		Expect(configAppliedCond.Type).To(Equal(string(utils.CRconditionTypes.ConfigurationApplied)))
		Expect(configAppliedCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(configAppliedCond.Reason).To(Equal(string(utils.CRconditionReasons.TimedOut)))
		Expect(configAppliedCond.Message).To(
			Equal("The configuration is still being applied, but it timed out"))
	})

	It("Updates ClusterRequest ConfigurationApplied condition to Missing if there are no policies", func() {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "cluster-1",
				Namespace: ctNamespace,
			},
		}

		result, err := CRReconciler.Reconcile(ctx, req)
		Expect(err).ToNot(HaveOccurred())
		// Expect to not requeue on valid cluster request.
		Expect(result.Requeue).To(BeFalse())

		clusterRequest := &oranv1alpha1.ClusterRequest{}

		// Create the ClusterRequest reconciliation task.
		err = CRReconciler.Client.Get(
			context.TODO(),
			types.NamespacedName{
				Name:      "cluster-1",
				Namespace: "clustertemplate-a-v4-16",
			},
			clusterRequest)
		Expect(err).ToNot(HaveOccurred())

		CRTask = &clusterRequestReconcilerTask{
			logger: CRReconciler.Logger,
			client: CRReconciler.Client,
			object: clusterRequest, // cluster-1 request
		}

		// Call the handleClusterPolicyConfiguration function.
		requeue, err := CRTask.handleClusterPolicyConfiguration(context.Background())
		Expect(requeue).To(BeFalse())
		Expect(err).ToNot(HaveOccurred())
		Expect(CRTask.object.Status.Policies).To(BeEmpty())
		Expect(CRTask.object.Status.ClusterInstanceRef.NonCompliantAt).To(BeZero())

		// Check the status conditions.
		conditions := CRTask.object.Status.Conditions
		configAppliedCond := meta.FindStatusCondition(
			conditions, string(utils.CRconditionTypes.ConfigurationApplied))
		Expect(configAppliedCond).ToNot(BeNil())
		Expect(configAppliedCond.Type).To(Equal(string(utils.CRconditionTypes.ConfigurationApplied)))
		Expect(configAppliedCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(configAppliedCond.Reason).To(Equal(string(utils.CRconditionReasons.Missing)))
	})

	It("It handles updated/deleted policies for matched clusters", func() {

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "cluster-1",
				Namespace: ctNamespace,
			},
		}

		result, err := CRReconciler.Reconcile(ctx, req)
		Expect(err).ToNot(HaveOccurred())
		// Expect to not requeue on valid cluster request.
		Expect(result.Requeue).To(BeFalse())
		// Expect the ClusterInstance and its namespace to have been created.
		clusterInstanceNs := &corev1.Namespace{}
		err = CRReconciler.Client.Get(
			context.TODO(),
			client.ObjectKey{Name: "cluster-1"},
			clusterInstanceNs,
		)
		Expect(err).ToNot(HaveOccurred())
		clusterInstance := &siteconfig.ClusterInstance{}
		err = CRReconciler.Client.Get(
			context.TODO(),
			types.NamespacedName{
				Name:      "cluster-1",
				Namespace: "cluster-1",
			},
			clusterInstance)
		Expect(err).ToNot(HaveOccurred())

		// Check updated policies for matched clusters result in reconciliation request.
		updateEvent = &event.UpdateEvent{
			ObjectOld: &policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ztp-clustertemplate-a-v4-16.policy",
					Namespace: "cluster-1",
				},
				Spec: policiesv1.PolicySpec{
					RemediationAction: "inform",
				},
				Status: policiesv1.PolicyStatus{
					ComplianceState: "Compliant",
				},
			},
			ObjectNew: &policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ztp-clustertemplate-a-v4-16.policy",
					Namespace: "cluster-1",
				},
				Spec: policiesv1.PolicySpec{
					RemediationAction: "enforce",
				},
				Status: policiesv1.PolicyStatus{
					ComplianceState: "Compliant",
				},
			},
		}
		queue := workqueue.NewRateLimitingQueueWithConfig(
			workqueue.DefaultControllerRateLimiter(),
			workqueue.RateLimitingQueueConfig{
				Name: "ClusterRequestsQueue",
			})
		CRReconciler.handlePolicyEventUpdate(ctx, *updateEvent, queue)
		Expect(queue.Len()).To(Equal(1))

		// Get the first request from the queue.
		item, shutdown := queue.Get()
		Expect(shutdown).To(BeFalse())

		Expect(item).To(Equal(
			reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      "cluster-1",
				Namespace: ctNamespace,
			}},
		))

		// Check that deleted policies for matched clusters result in reconciliation requests.
		deleteEvent := &event.DeleteEvent{
			Object: &policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ztp-clustertemplate-a-v4-16.policy",
					Namespace: "cluster-1",
				},
				Spec: policiesv1.PolicySpec{
					RemediationAction: "inform",
				},
				Status: policiesv1.PolicyStatus{
					ComplianceState: "Compliant",
				},
			},
		}
		queue = workqueue.NewRateLimitingQueueWithConfig(
			workqueue.DefaultControllerRateLimiter(),
			workqueue.RateLimitingQueueConfig{
				Name: "ClusterRequestsQueue",
			})
		CRReconciler.handlePolicyEventDelete(ctx, *deleteEvent, queue)
		Expect(queue.Len()).To(Equal(1))

		// Get the first request from the queue.
		item, shutdown = queue.Get()
		Expect(shutdown).To(BeFalse())

		Expect(item).To(Equal(
			reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      "cluster-1",
				Namespace: ctNamespace,
			}},
		))
	})

	It("Updates ClusterRequest ConfigurationApplied condition to OutOfDate when the cluster is "+
		"NonCompliant with at least one matched policies and the policy is not in enforce", func() {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "cluster-1",
				Namespace: ctNamespace,
			},
		}

		result, err := CRReconciler.Reconcile(ctx, req)
		Expect(err).ToNot(HaveOccurred())
		// Expect to not requeue on valid cluster request.
		Expect(result.Requeue).To(BeFalse())

		newPolicies := []client.Object{
			&policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ztp-clustertemplate-a-v4-16.v1-subscriptions-policy",
					Namespace: "cluster-1",
					Labels: map[string]string{
						utils.ChildPolicyRootPolicyLabel:       "ztp-clustertemplate-a-v4-16.v1-subscriptions-policy",
						utils.ChildPolicyClusterNameLabel:      "cluster-1",
						utils.ChildPolicyClusterNamespaceLabel: "cluster-1",
					},
				},
				Spec: policiesv1.PolicySpec{
					RemediationAction: "inform",
				},
				Status: policiesv1.PolicyStatus{
					ComplianceState: "NonCompliant",
				},
			},
			&policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ztp-clustertemplate-a-v4-16.v1-sriov-configuration-policy",
					Namespace: "cluster-1",
					Labels: map[string]string{
						utils.ChildPolicyRootPolicyLabel:       "ztp-clustertemplate-a-v4-16.v1-sriov-configuration-policy",
						utils.ChildPolicyClusterNameLabel:      "cluster-1",
						utils.ChildPolicyClusterNamespaceLabel: "cluster-1",
					},
				},
				Spec: policiesv1.PolicySpec{
					RemediationAction: "enforce",
				},
				Status: policiesv1.PolicyStatus{
					ComplianceState: "Compliant",
				},
			},
		}
		// Create all the ACM policies.
		for _, newPolicy := range newPolicies {
			Expect(c.Create(ctx, newPolicy)).To(Succeed())
		}
		clusterRequest := &oranv1alpha1.ClusterRequest{}

		// Create the ClusterRequest reconciliation task.
		err = CRReconciler.Client.Get(
			context.TODO(),
			types.NamespacedName{
				Name:      "cluster-1",
				Namespace: "clustertemplate-a-v4-16",
			},
			clusterRequest)
		Expect(err).ToNot(HaveOccurred())

		CRTask = &clusterRequestReconcilerTask{
			logger: CRReconciler.Logger,
			client: CRReconciler.Client,
			object: clusterRequest, // cluster-1 request
		}

		// Call the handleClusterPolicyConfiguration function.
		requeue, err := CRTask.handleClusterPolicyConfiguration(context.Background())
		Expect(requeue).To(BeFalse())
		Expect(err).ToNot(HaveOccurred())
		Expect(CRTask.object.Status.Policies).To(ConsistOf(
			[]oranv1alpha1.PolicyDetails{
				{
					Compliant:         "Compliant",
					PolicyName:        "v1-sriov-configuration-policy",
					PolicyNamespace:   "ztp-clustertemplate-a-v4-16",
					RemediationAction: "enforce",
				},
				{
					Compliant:         "NonCompliant",
					PolicyName:        "v1-subscriptions-policy",
					PolicyNamespace:   "ztp-clustertemplate-a-v4-16",
					RemediationAction: "inform",
				},
			},
		))

		// Check the status conditions.
		conditions := CRTask.object.Status.Conditions
		configAppliedCond := meta.FindStatusCondition(
			conditions, string(utils.CRconditionTypes.ConfigurationApplied))
		Expect(configAppliedCond).ToNot(BeNil())
		Expect(configAppliedCond.Type).To(Equal(string(utils.CRconditionTypes.ConfigurationApplied)))
		Expect(configAppliedCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(configAppliedCond.Reason).To(Equal(string(utils.CRconditionReasons.OutOfDate)))
		Expect(configAppliedCond.Message).To(Equal("The configuration is out of date"))
	})

	It("Updates ClusterRequest ConfigurationApplied condition to Completed when the cluster is "+
		"Compliant with all the matched policies", func() {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "cluster-1",
				Namespace: ctNamespace,
			},
		}

		result, err := CRReconciler.Reconcile(ctx, req)
		Expect(err).ToNot(HaveOccurred())
		// Expect to not requeue on valid cluster request.
		Expect(result.Requeue).To(BeFalse())

		newPolicies := []client.Object{
			&policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ztp-clustertemplate-a-v4-16.v1-subscriptions-policy",
					Namespace: "cluster-1",
					Labels: map[string]string{
						utils.ChildPolicyRootPolicyLabel:       "ztp-clustertemplate-a-v4-16.v1-subscriptions-policy",
						utils.ChildPolicyClusterNameLabel:      "cluster-1",
						utils.ChildPolicyClusterNamespaceLabel: "cluster-1",
					},
				},
				Spec: policiesv1.PolicySpec{
					RemediationAction: "inform",
				},
				Status: policiesv1.PolicyStatus{
					ComplianceState: "Compliant",
				},
			},
			&policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ztp-clustertemplate-a-v4-16.v1-sriov-configuration-policy",
					Namespace: "cluster-1",
					Labels: map[string]string{
						utils.ChildPolicyRootPolicyLabel:       "ztp-clustertemplate-a-v4-16.v1-sriov-configuration-policy",
						utils.ChildPolicyClusterNameLabel:      "cluster-1",
						utils.ChildPolicyClusterNamespaceLabel: "cluster-1",
					},
				},
				Spec: policiesv1.PolicySpec{
					RemediationAction: "enforce",
				},
				Status: policiesv1.PolicyStatus{
					ComplianceState: "Compliant",
				},
			},
		}
		// Create all the ACM policies.
		for _, newPolicy := range newPolicies {
			Expect(c.Create(ctx, newPolicy)).To(Succeed())
		}
		clusterRequest := &oranv1alpha1.ClusterRequest{}

		// Create the ClusterRequest reconciliation task.
		err = CRReconciler.Client.Get(
			context.TODO(),
			types.NamespacedName{
				Name:      "cluster-1",
				Namespace: "clustertemplate-a-v4-16",
			},
			clusterRequest)
		Expect(err).ToNot(HaveOccurred())

		CRTask = &clusterRequestReconcilerTask{
			logger: CRReconciler.Logger,
			client: CRReconciler.Client,
			object: clusterRequest, // cluster-1 request
		}

		// Call the handleClusterPolicyConfiguration function.
		requeue, err := CRTask.handleClusterPolicyConfiguration(context.Background())
		Expect(requeue).To(BeFalse())
		Expect(err).ToNot(HaveOccurred())
		Expect(CRTask.object.Status.Policies).To(ConsistOf(
			[]oranv1alpha1.PolicyDetails{
				{
					Compliant:         "Compliant",
					PolicyName:        "v1-sriov-configuration-policy",
					PolicyNamespace:   "ztp-clustertemplate-a-v4-16",
					RemediationAction: "enforce",
				},
				{
					Compliant:         "Compliant",
					PolicyName:        "v1-subscriptions-policy",
					PolicyNamespace:   "ztp-clustertemplate-a-v4-16",
					RemediationAction: "inform",
				},
			},
		))

		// Check the status conditions.
		conditions := CRTask.object.Status.Conditions
		configAppliedCond := meta.FindStatusCondition(
			conditions, string(utils.CRconditionTypes.ConfigurationApplied))
		Expect(configAppliedCond).ToNot(BeNil())
		Expect(configAppliedCond.Type).To(Equal(string(utils.CRconditionTypes.ConfigurationApplied)))
		Expect(configAppliedCond.Status).To(Equal(metav1.ConditionTrue))
		Expect(configAppliedCond.Reason).To(Equal(string(utils.CRconditionReasons.Completed)))
		Expect(configAppliedCond.Message).To(Equal("The configuration is up to date"))
	})

	It("Updates ClusterRequest ConfigurationApplied condition to InProgress when the cluster is "+
		"NonCompliant with at least one enforce policy", func() {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "cluster-1",
				Namespace: ctNamespace,
			},
		}

		result, err := CRReconciler.Reconcile(ctx, req)
		Expect(err).ToNot(HaveOccurred())
		// Expect to not requeue on valid cluster request.
		Expect(result.Requeue).To(BeFalse())

		newPolicies := []client.Object{
			&policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ztp-clustertemplate-a-v4-16.v1-subscriptions-policy",
					Namespace: "cluster-1",
					Labels: map[string]string{
						utils.ChildPolicyRootPolicyLabel:       "ztp-clustertemplate-a-v4-16.v1-subscriptions-policy",
						utils.ChildPolicyClusterNameLabel:      "cluster-1",
						utils.ChildPolicyClusterNamespaceLabel: "cluster-1",
					},
				},
				Spec: policiesv1.PolicySpec{
					RemediationAction: "inform",
				},
				Status: policiesv1.PolicyStatus{
					ComplianceState: "Compliant",
				},
			},
			&policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ztp-clustertemplate-a-v4-16.v1-sriov-configuration-policy",
					Namespace: "cluster-1",
					Labels: map[string]string{
						utils.ChildPolicyRootPolicyLabel:       "ztp-clustertemplate-a-v4-16.v1-sriov-configuration-policy",
						utils.ChildPolicyClusterNameLabel:      "cluster-1",
						utils.ChildPolicyClusterNamespaceLabel: "cluster-1",
					},
				},
				Spec: policiesv1.PolicySpec{
					RemediationAction: "enforce",
				},
				Status: policiesv1.PolicyStatus{
					ComplianceState: "NonCompliant",
				},
			},
		}
		// Create all the ACM policies.
		for _, newPolicy := range newPolicies {
			Expect(c.Create(ctx, newPolicy)).To(Succeed())
		}
		clusterRequest := &oranv1alpha1.ClusterRequest{}

		// Create the ClusterRequest reconciliation task.
		err = CRReconciler.Client.Get(
			context.TODO(),
			types.NamespacedName{
				Name:      "cluster-1",
				Namespace: "clustertemplate-a-v4-16",
			},
			clusterRequest)
		Expect(err).ToNot(HaveOccurred())

		CRTask = &clusterRequestReconcilerTask{
			logger: CRReconciler.Logger,
			client: CRReconciler.Client,
			object: clusterRequest, // cluster-1 request
		}

		// Call the handleClusterPolicyConfiguration function.
		requeue, err := CRTask.handleClusterPolicyConfiguration(context.Background())
		Expect(requeue).To(BeTrue())
		Expect(err).ToNot(HaveOccurred())
		Expect(CRTask.object.Status.Policies).To(ConsistOf(
			[]oranv1alpha1.PolicyDetails{
				{
					Compliant:         "NonCompliant",
					PolicyName:        "v1-sriov-configuration-policy",
					PolicyNamespace:   "ztp-clustertemplate-a-v4-16",
					RemediationAction: "enforce",
				},
				{
					Compliant:         "Compliant",
					PolicyName:        "v1-subscriptions-policy",
					PolicyNamespace:   "ztp-clustertemplate-a-v4-16",
					RemediationAction: "inform",
				},
			},
		))

		// Check the status conditions.
		conditions := CRTask.object.Status.Conditions
		configAppliedCond := meta.FindStatusCondition(
			conditions, string(utils.CRconditionTypes.ConfigurationApplied))
		Expect(configAppliedCond).ToNot(BeNil())
		Expect(configAppliedCond.Type).To(Equal(string(utils.CRconditionTypes.ConfigurationApplied)))
		Expect(configAppliedCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(configAppliedCond.Reason).To(Equal(string(utils.CRconditionReasons.InProgress)))
		Expect(configAppliedCond.Message).To(Equal("The configuration is still being applied"))
	})

	It("Updates ClusterRequest ConfigurationApplied condition to InProgress when the cluster is "+
		"Pending with at least one enforce policy", func() {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "cluster-1",
				Namespace: ctNamespace,
			},
		}

		result, err := CRReconciler.Reconcile(ctx, req)
		Expect(err).ToNot(HaveOccurred())
		// Expect to not requeue on valid cluster request.
		Expect(result.Requeue).To(BeFalse())

		newPolicies := []client.Object{
			&policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ztp-clustertemplate-a-v4-16.v1-subscriptions-policy",
					Namespace: "cluster-1",
					Labels: map[string]string{
						utils.ChildPolicyRootPolicyLabel:       "ztp-clustertemplate-a-v4-16.v1-subscriptions-policy",
						utils.ChildPolicyClusterNameLabel:      "cluster-1",
						utils.ChildPolicyClusterNamespaceLabel: "cluster-1",
					},
				},
				Spec: policiesv1.PolicySpec{
					RemediationAction: "inform",
				},
				Status: policiesv1.PolicyStatus{
					ComplianceState: "Compliant",
				},
			},
			&policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ztp-clustertemplate-a-v4-16.v1-sriov-configuration-policy",
					Namespace: "cluster-1",
					Labels: map[string]string{
						utils.ChildPolicyRootPolicyLabel:       "ztp-clustertemplate-a-v4-16.v1-sriov-configuration-policy",
						utils.ChildPolicyClusterNameLabel:      "cluster-1",
						utils.ChildPolicyClusterNamespaceLabel: "cluster-1",
					},
				},
				Spec: policiesv1.PolicySpec{
					RemediationAction: "enforce",
				},
				Status: policiesv1.PolicyStatus{
					ComplianceState: "Pending",
				},
			},
		}
		// Create all the ACM policies.
		for _, newPolicy := range newPolicies {
			Expect(c.Create(ctx, newPolicy)).To(Succeed())
		}
		clusterRequest := &oranv1alpha1.ClusterRequest{}

		// Create the ClusterRequest reconciliation task.
		err = CRReconciler.Client.Get(
			context.TODO(),
			types.NamespacedName{
				Name:      "cluster-1",
				Namespace: "clustertemplate-a-v4-16",
			},
			clusterRequest)
		Expect(err).ToNot(HaveOccurred())

		CRTask = &clusterRequestReconcilerTask{
			logger: CRReconciler.Logger,
			client: CRReconciler.Client,
			object: clusterRequest, // cluster-1 request
		}

		// Call the handleClusterPolicyConfiguration function.
		requeue, err := CRTask.handleClusterPolicyConfiguration(context.Background())
		Expect(requeue).To(BeTrue())
		Expect(err).ToNot(HaveOccurred())
		Expect(CRTask.object.Status.Policies).To(ConsistOf(
			[]oranv1alpha1.PolicyDetails{
				{
					Compliant:         "Pending",
					PolicyName:        "v1-sriov-configuration-policy",
					PolicyNamespace:   "ztp-clustertemplate-a-v4-16",
					RemediationAction: "enforce",
				},
				{
					Compliant:         "Compliant",
					PolicyName:        "v1-subscriptions-policy",
					PolicyNamespace:   "ztp-clustertemplate-a-v4-16",
					RemediationAction: "inform",
				},
			},
		))

		// Check the status conditions.
		conditions := CRTask.object.Status.Conditions
		configAppliedCond := meta.FindStatusCondition(
			conditions, string(utils.CRconditionTypes.ConfigurationApplied))
		Expect(configAppliedCond).ToNot(BeNil())
		Expect(configAppliedCond.Type).To(Equal(string(utils.CRconditionTypes.ConfigurationApplied)))
		Expect(configAppliedCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(configAppliedCond.Reason).To(Equal(string(utils.CRconditionReasons.InProgress)))
		Expect(configAppliedCond.Message).To(Equal("The configuration is still being applied"))
	})
})

var _ = Describe("hasPolicyConfigurationTimedOut", func() {
	var (
		ctx          context.Context
		c            client.Client
		CRReconciler *ClusterRequestReconciler
		CRTask       *clusterRequestReconcilerTask
		CTReconciler *ClusterTemplateReconciler
		ctName       = "clustertemplate-a-v1"
		ctNamespace  = "clustertemplate-a-v4-16"
	)

	BeforeEach(func() {
		// Define the needed resources.
		crs := []client.Object{
			// Cluster Template Namespace.
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: ctNamespace,
				},
			},
			// Cluster Request.
			&oranv1alpha1.ClusterRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "cluster-1",
					Namespace:  ctNamespace,
					Finalizers: []string{clusterRequestFinalizer},
				},
				Spec: oranv1alpha1.ClusterRequestSpec{
					ClusterTemplateRef: ctName,
					ClusterTemplateInput: oranv1alpha1.ClusterTemplateInput{
						ClusterInstanceInput: runtime.RawExtension{
							Raw: []byte(testClusterTemplateInput),
						},
						PolicyTemplateInput: runtime.RawExtension{
							Raw: []byte(testPolicyTemplateInput),
						},
					},
					Timeout: oranv1alpha1.Timeout{
						Configuration: 1,
					},
				},
				Status: oranv1alpha1.ClusterRequestStatus{
					ClusterInstanceRef: &oranv1alpha1.ClusterInstanceRef{},
				},
			},
		}

		c = getFakeClientFromObjects(crs...)
		// Reconcile the ClusterTemplate.
		CTReconciler = &ClusterTemplateReconciler{
			Client: c,
			Logger: logger,
		}

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      ctName,
				Namespace: ctNamespace,
			},
		}

		_, err := CTReconciler.Reconcile(ctx, req)
		Expect(err).ToNot(HaveOccurred())

		CRReconciler = &ClusterRequestReconciler{
			Client: c,
			Logger: logger,
		}

		CRTask = &clusterRequestReconcilerTask{
			logger: CRReconciler.Logger,
			client: CRReconciler.Client,
			object: crs[1].(*oranv1alpha1.ClusterRequest), // cluster-1 request
		}
	})

	It("Returns false if the status is unexpected and NonCompliantAt is not set", func() {
		// Set the status to InProgress.
		utils.SetStatusCondition(&CRTask.object.Status.Conditions,
			utils.CRconditionTypes.ConfigurationApplied,
			utils.CRconditionReasons.Unknown,
			metav1.ConditionFalse,
			"",
		)
		// Start from empty NonCompliantAt.
		Expect(CRTask.object.Status.ClusterInstanceRef.NonCompliantAt.Time).To(BeZero())
		policyTimedOut := CRTask.hasPolicyConfigurationTimedOut(ctx)
		// Check that NonCompliantAt was set and that the return is false.
		Expect(policyTimedOut).To(BeFalse())
		Expect(CRTask.object.Status.ClusterInstanceRef.NonCompliantAt.Time).To(BeZero())
	})

	It("Returns false if the status is Completed and sets NonCompliantAt", func() {
		// Set the status to InProgress.
		utils.SetStatusCondition(&CRTask.object.Status.Conditions,
			utils.CRconditionTypes.ConfigurationApplied,
			utils.CRconditionReasons.Completed,
			metav1.ConditionTrue,
			"",
		)
		// Start from empty NonCompliantAt.
		Expect(CRTask.object.Status.ClusterInstanceRef.NonCompliantAt.Time).To(BeZero())
		policyTimedOut := CRTask.hasPolicyConfigurationTimedOut(ctx)
		// Check that NonCompliantAt was set and that the return is false.
		Expect(policyTimedOut).To(BeFalse())
		Expect(CRTask.object.Status.ClusterInstanceRef.NonCompliantAt.Time).ToNot(BeZero())
	})

	It("Returns false if the status is OutOfDate and sets NonCompliantAt", func() {
		// Set the status to InProgress.
		utils.SetStatusCondition(&CRTask.object.Status.Conditions,
			utils.CRconditionTypes.ConfigurationApplied,
			utils.CRconditionReasons.OutOfDate,
			metav1.ConditionFalse,
			"",
		)
		// Start from empty NonCompliantAt.
		Expect(CRTask.object.Status.ClusterInstanceRef.NonCompliantAt.Time).To(BeZero())
		policyTimedOut := CRTask.hasPolicyConfigurationTimedOut(ctx)
		// Check that NonCompliantAt was set and that the return is false.
		Expect(policyTimedOut).To(BeFalse())
		Expect(CRTask.object.Status.ClusterInstanceRef.NonCompliantAt.Time).ToNot(BeZero())
	})

	It("Returns false if the status is Missing and sets NonCompliantAt", func() {
		// Set the status to InProgress.
		utils.SetStatusCondition(&CRTask.object.Status.Conditions,
			utils.CRconditionTypes.ConfigurationApplied,
			utils.CRconditionReasons.Missing,
			metav1.ConditionFalse,
			"",
		)
		// Start from empty NonCompliantAt.
		Expect(CRTask.object.Status.ClusterInstanceRef.NonCompliantAt.Time).To(BeZero())
		policyTimedOut := CRTask.hasPolicyConfigurationTimedOut(ctx)
		// Check that NonCompliantAt was set and that the return is false.
		Expect(policyTimedOut).To(BeFalse())
		Expect(CRTask.object.Status.ClusterInstanceRef.NonCompliantAt.Time).ToNot(BeZero())
	})

	It("Returns true if the status is InProgress and the timeout has passed", func() {
		// Set the status to InProgress.
		utils.SetStatusCondition(&CRTask.object.Status.Conditions,
			utils.CRconditionTypes.ConfigurationApplied,
			utils.CRconditionReasons.InProgress,
			metav1.ConditionFalse,
			"",
		)
		// Set NonCompliantAt.
		nonCompliantAt := metav1.Now().Add(-2 * time.Minute)
		CRTask.object.Status.ClusterInstanceRef.NonCompliantAt.Time = nonCompliantAt
		policyTimedOut := CRTask.hasPolicyConfigurationTimedOut(ctx)
		// Check that NonCompliantAt wasn't changed and that the return is true.
		Expect(policyTimedOut).To(BeTrue())
		Expect(CRTask.object.Status.ClusterInstanceRef.NonCompliantAt.Time).To(Equal(nonCompliantAt))
	})

	It("Sets NonCompliantAt if there is no ConfigurationApplied condition", func() {
		policyTimedOut := CRTask.hasPolicyConfigurationTimedOut(ctx)
		Expect(policyTimedOut).To(BeFalse())
		Expect(CRTask.object.Status.ClusterInstanceRef.NonCompliantAt).ToNot(BeZero())
	})
})

/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package controllersE2Etest

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	siteconfig "github.com/stolostron/siteconfig/api/v1alpha1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	ibgu "github.com/openshift-kni/cluster-group-upgrades-operator/pkg/api/imagebasedgroupupgrades/v1alpha1"
	"github.com/openshift-kni/oran-o2ims/api/common"
	pluginsv1alpha1 "github.com/openshift-kni/oran-o2ims/api/hardwaremanagement/plugins/v1alpha1"
	hwmgmtv1alpha1 "github.com/openshift-kni/oran-o2ims/api/hardwaremanagement/v1alpha1"
	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	hwmgrutils "github.com/openshift-kni/oran-o2ims/hwmgr-plugins/controller/utils"
	metal3pluginscontrollers "github.com/openshift-kni/oran-o2ims/hwmgr-plugins/metal3/controller"
	"github.com/openshift-kni/oran-o2ims/internal/constants"
	provisioningcontrollers "github.com/openshift-kni/oran-o2ims/internal/controllers"
	ctlrutils "github.com/openshift-kni/oran-o2ims/internal/controllers/utils"
	testutils "github.com/openshift-kni/oran-o2ims/test/utils"
	assistedservicev1beta1 "github.com/openshift/assisted-service/api/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

const testHwMgrPluginNameSpace = "hwmgr"
const testHardwarePluginRef = "hwmgr"

var (
	K8SClient                           client.Client
	K8SManager                          ctrl.Manager
	Metal3Manager                       ctrl.Manager
	ProvReqTestReconciler               *provisioningcontrollers.ProvisioningRequestReconciler
	ClusterTemplateTestReconciler       *provisioningcontrollers.ClusterTemplateReconciler
	NodeAllocationRequestTestReconciler *metal3pluginscontrollers.NodeAllocationRequestReconciler
	AllocatedNodeTestReconciler         *metal3pluginscontrollers.AllocatedNodeReconciler
	testEnv                             *envtest.Environment
	ctx                                 context.Context
	cancel                              context.CancelFunc
	// store external CRDs
	tmpDir string
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	tmpDir = t.TempDir()
	RunSpecs(t, "Controllers end to end")
}

// Logger used for tests:
var logger *slog.Logger

var _ = BeforeSuite(func() {
	// Create a logger that writes to the Ginkgo writer, so that the log messages will be
	// attached to the output of the right test:
	options := &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	handler := slog.NewJSONHandler(GinkgoWriter, options)
	logger = slog.New(handler)

	// Configure the controller runtime library to use our logger:
	adapter := logr.FromSlogHandler(logger.Handler())
	ctrl.SetLogger(adapter)
	klog.SetLogger(adapter)

	// Set the hardware manager plugin details.
	os.Setenv(ctlrutils.HwMgrPluginNameSpace, testHwMgrPluginNameSpace)

	// Set the scheme.
	testScheme := runtime.NewScheme()
	err := provisioningv1alpha1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = hwmgmtv1alpha1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = corev1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = siteconfig.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = policiesv1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = clusterv1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = assistedservicev1beta1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = apiextensionsv1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = admissionregistrationv1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = ibgu.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = policiesv1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = clusterv1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = pluginsv1alpha1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = hivev1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = metal3v1alpha1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())

	// Get the needed external CRDs. Their details are under test/utils/vars.go - ExternalCrdsData.
	// Update that with any other CRDs that the provisioning controller depends on.
	Expect(testutils.GetExternalCrdFiles(tmpDir)).To(Succeed())
	// Start testEnv - include the directories holding the external CRDs.
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "vendor", "open-cluster-management.io", "api", "cluster", "v1"),
			tmpDir,
		},
		ErrorIfCRDPathMissing: true,
		Scheme:                testScheme,
	}
	// Start testEnv.
	cfg, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())
	ctx, cancel = context.WithCancel(context.TODO())

	// Get the main manager for O2IMS controllers.
	K8SManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: testScheme,
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(K8SManager).NotTo(BeNil())

	// Get a separate manager for Metal3 controllers (simulates separate pod deployment).
	Metal3Manager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: testScheme,
		Metrics: metricsserver.Options{
			BindAddress: ":8081", // Use different port to avoid conflict
		},
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(Metal3Manager).NotTo(BeNil())

	// Get the client.
	K8SClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(K8SClient).NotTo(BeNil())

	// Setup the ClusterTemplate Reconciler.
	ClusterTemplateTestReconciler = &provisioningcontrollers.ClusterTemplateReconciler{
		Client: K8SClient,
		Logger: logger,
	}
	err = ClusterTemplateTestReconciler.SetupWithManager(K8SManager)
	Expect(err).ToNot(HaveOccurred())

	// Initialize NodeAllocationRequest utils for Metal3 controllers
	err = hwmgrutils.InitNodeAllocationRequestUtils(testScheme)
	Expect(err).ToNot(HaveOccurred())

	// Setup Metal3 controllers on separate manager (simulates separate pod deployment)
	metal3controllers, err := metal3pluginscontrollers.SetupMetal3Controllers(Metal3Manager, testHwMgrPluginNameSpace, logger)
	Expect(err).ToNot(HaveOccurred())
	NodeAllocationRequestTestReconciler = metal3controllers.NodeAllocationReconciler
	AllocatedNodeTestReconciler = metal3controllers.AllocatedNodeReconciler

	// Setup the ProvisioningRequest Reconciler on main manager.
	ProvReqTestReconciler = &provisioningcontrollers.ProvisioningRequestReconciler{
		Client:         K8SClient,
		Logger:         logger,
		CallbackConfig: ctlrutils.NewNarCallbackConfig(constants.DefaultNarCallbackServicePort),
	}
	err = ProvReqTestReconciler.SetupWithManager(K8SManager)
	Expect(err).ToNot(HaveOccurred())

	// Start mock hardware plugin server for e2e tests with Kubernetes client
	mockServer := provisioningcontrollers.NewMockHardwarePluginServerWithClient(K8SClient)

	suiteCrs := []client.Object{
		// HW plugin test namespace
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ctlrutils.UnitTestHwmgrNamespace,
			},
		},
		// oran-o2ims
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: constants.DefaultNamespace,
			},
		},
		// Basic auth secret for hardware plugin
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-hwmgr-auth-secret",
				Namespace: testHwMgrPluginNameSpace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"username": []byte("test-user"),
				"password": []byte("test-password"),
			},
		},
		// HardwarePlugin CRs - must be in HWMGR_PLUGIN_NAMESPACE where controller looks for it
		&hwmgmtv1alpha1.HardwarePlugin{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testHwMgrPluginNameSpace,
				Name:      testHardwarePluginRef,
			},
			Spec: hwmgmtv1alpha1.HardwarePluginSpec{
				ApiRoot: mockServer.GetURL(),
				AuthClientConfig: &common.AuthClientConfig{
					Type:            common.Basic,
					BasicAuthSecret: stringPtr("test-hwmgr-auth-secret"),
				},
			},
		},
	}

	for _, cr := range suiteCrs {
		err := K8SClient.Create(context.Background(), cr)
		Expect(err).ToNot(HaveOccurred())
	}

	// Update HardwarePlugin status to mark it as registered
	mockHwPlugin := &hwmgmtv1alpha1.HardwarePlugin{}
	err = K8SClient.Get(context.Background(), types.NamespacedName{
		Namespace: testHwMgrPluginNameSpace,
		Name:      testHardwarePluginRef,
	}, mockHwPlugin)
	Expect(err).ToNot(HaveOccurred())

	mockHwPlugin.Status.Conditions = []metav1.Condition{
		{
			Type:               string(hwmgmtv1alpha1.ConditionTypes.Registration),
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             string(hwmgmtv1alpha1.ConditionReasons.Completed),
			Message:            "Mock HardwarePlugin registered successfully for e2e tests",
		},
	}
	err = K8SClient.Status().Update(context.Background(), mockHwPlugin)
	Expect(err).ToNot(HaveOccurred())

	NodeAllocationRequestTestReconciler.InitializeCallbackContext(ctx)

	// Start the main O2IMS manager
	go func() {
		defer GinkgoRecover()
		err = K8SManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run main manager")
	}()

	// Start the Metal3 manager
	go func() {
		defer GinkgoRecover()
		err = Metal3Manager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run Metal3 manager")
	}()
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// stringPtr is a helper function to get a pointer to a string
func stringPtr(s string) *string {
	return &s
}

var _ = Describe("Dry-run-ProvisioningRequestReconcile", func() {
	const timeout = time.Second * 60
	const interval = time.Second * 3

	var (
		testCtx                context.Context
		ProvRequestCR          *provisioningv1alpha1.ProvisioningRequest
		ctIncomplete           *provisioningv1alpha1.ClusterTemplate
		ctComplete             *provisioningv1alpha1.ClusterTemplate
		tName                  = "clustertemplate-a"
		tVersion1              = "v1.0.0"
		tVersion2              = "v2.0.0"
		ctNamespace            = "clustertemplate-a-v4-16"
		ciDefaultsCmIncomplete = "clusterinstance-defaults-v1"
		ciDefaultsCmComplete   = "clusterinstance-defaults-v2"
		ptDefaultsCm           = "policytemplate-defaults-v1"
		hwTemplate             = "hwtemplate-v1"
	)

	testCtx = context.Background()

	mainCRs := []client.Object{
		// Cluster Template Namespace
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ctNamespace,
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ztp-" + ctNamespace,
			},
		},
		// Configmap for ClusterInstance defaults v1 - missing required values.
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ciDefaultsCmIncomplete,
				Namespace: ctNamespace,
			},
			Data: map[string]string{
				ctlrutils.ClusterInstallationTimeoutConfigKey: "60s",
				ctlrutils.ClusterInstanceTemplateDefaultsConfigmapKey: `
clusterImageSetNameRef: "4.15.0"
holdInstallation: false
cpuPartitioningMode: AllNodes
networkType: OVNKubernetes
pullSecretRef:
  name: "pull-secret"
templateRefs:
- name: "ai-cluster-templates-v1"
  namespace: "siteconfig-operator"
nodes:
- role: master
  automatedCleaningMode: disabled
  ironicInspect: ""
  bootMode: UEFI
  nodeNetwork:
    interfaces:
    - name: eno1
      label: bootable-interface
    - name: eth0
      label: base-interface
    - name: eth1
      label: data-interface
`,
			},
		},
		// Configmap for ClusterInstance defaults - complete.
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ciDefaultsCmComplete,
				Namespace: ctNamespace,
			},
			Data: map[string]string{
				ctlrutils.ClusterInstallationTimeoutConfigKey: "60s",
				ctlrutils.ClusterInstanceTemplateDefaultsConfigmapKey: `
clusterImageSetNameRef: "4.15.0"
holdInstallation: false
cpuPartitioningMode: AllNodes
networkType: OVNKubernetes
pullSecretRef:
  name: "pull-secret"
templateRefs:
- name: "ai-cluster-templates-v1"
  namespace: "siteconfig-operator"
nodes:
- role: master
  automatedCleaningMode: disabled
  ironicInspect: ""
  bootMode: UEFI
  hostName: node1
  nodeNetwork:
    interfaces:
    - name: eno1
      label: bootable-interface
    - name: eth0
      label: base-interface
    - name: eth1
      label: data-interface
  templateRefs:
  - name: test
    namespace: test
`,
			},
		},
		// Configmap for policy template defaults
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ptDefaultsCm,
				Namespace: ctNamespace,
			},
			Data: map[string]string{
				ctlrutils.ClusterConfigurationTimeoutConfigKey: "1m",
				ctlrutils.PolicyTemplateDefaultsConfigmapKey: `
cpu-isolated: "2-31"
cpu-reserved: "0-1"
defaultHugepagesSize: "1G"`,
			},
		},
		// hardware template
		&hwmgmtv1alpha1.HardwareTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      hwTemplate,
				Namespace: ctlrutils.InventoryNamespace,
			},
			Spec: hwmgmtv1alpha1.HardwareTemplateSpec{
				HardwarePluginRef:           ctlrutils.UnitTestHwPluginRef,
				BootInterfaceLabel:          "bootable-interface",
				HardwareProvisioningTimeout: "1m",
				NodeGroupData: []hwmgmtv1alpha1.NodeGroupData{
					{
						Name:           "controller",
						Role:           "master",
						ResourcePoolId: "xyz",
						HwProfile:      "profile-spr-single-processor-64G",
					},
					{
						Name:           "worker",
						Role:           "worker",
						ResourcePoolId: "xyz",
						HwProfile:      "profile-spr-dual-processor-128G",
					},
				},
			},
		},
		// Pull secret for ClusterInstance
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pull-secret",
				Namespace: ctNamespace,
			},
			Data: map[string][]byte{
				".dockerconfigjson": []byte(testutils.TestSecretDataStr),
			},
			Type: corev1.SecretTypeDockerConfigJson,
		},
		// ClusterImageSet for e2e tests
		&hivev1.ClusterImageSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "4.15.0",
			},
			Spec: hivev1.ClusterImageSetSpec{
				ReleaseImage: "quay.io/openshift-release-dev/ocp-release:4.15.0-x86_64",
			},
		},
	}
	// Define the cluster templates.
	ctIncomplete = &provisioningv1alpha1.ClusterTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      provisioningcontrollers.GetClusterTemplateRefName(tName, tVersion1),
			Namespace: ctNamespace,
		},
		Spec: provisioningv1alpha1.ClusterTemplateSpec{
			Name:       tName,
			Version:    tVersion1,
			Release:    "4.15.0",
			TemplateID: "aab39bda-ac56-4143-9b10-d1a71517d04f",
			Templates: provisioningv1alpha1.Templates{
				ClusterInstanceDefaults: ciDefaultsCmIncomplete,
				PolicyTemplateDefaults:  ptDefaultsCm,
				HwTemplate:              hwTemplate,
			},
			TemplateParameterSchema: runtime.RawExtension{Raw: []byte(testutils.TestFullTemplateSchema)},
		},
	}
	ctComplete = &provisioningv1alpha1.ClusterTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      provisioningcontrollers.GetClusterTemplateRefName(tName, tVersion2),
			Namespace: ctNamespace,
		},
		Spec: provisioningv1alpha1.ClusterTemplateSpec{
			Name:       tName,
			Version:    tVersion2,
			Release:    "4.15.0",
			TemplateID: "bbb39bda-ac56-4143-9b10-d1a71517d04f",
			Templates: provisioningv1alpha1.Templates{
				ClusterInstanceDefaults: ciDefaultsCmComplete,
				PolicyTemplateDefaults:  ptDefaultsCm,
				HwTemplate:              hwTemplate,
			},
			TemplateParameterSchema: runtime.RawExtension{Raw: []byte(testutils.TestFullTemplateSchema)},
		},
	}
	ctCRs := []client.Object{ctComplete, ctIncomplete}

	// Define the provisioning request.
	ProvRequestCR = &provisioningv1alpha1.ProvisioningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Finalizers: []string{provisioningv1alpha1.ProvisioningRequestFinalizer},
			// Name to be set up in each test.
		},
		Spec: provisioningv1alpha1.ProvisioningRequestSpec{
			Name:            "test",
			Description:     "description",
			TemplateName:    tName,
			TemplateVersion: tVersion1,
		},
	}

	AfterEach(func() {
		for _, cr := range mainCRs {
			// Deleting namespaces is not supported.
			if _, ok := cr.(*corev1.Namespace); !ok {
				err := K8SClient.Delete(testCtx, cr)
				Expect(err).ToNot(HaveOccurred())
			}
		}

		//for _, cr := range ctCRs {
		//	Expect(K8SClient.Delete(testCtx, cr)).To(Succeed())
		//}

		/*
			// Clean up any ProvisioningRequests created during the tests
			provisioningRequests := &provisioningv1alpha1.ProvisioningRequestList{}
			err := K8SClient.List(testCtx, provisioningRequests)
			if err == nil {
				for _, pr := range provisioningRequests.Items {
					// Remove finalizers to allow deletion
					prCopy := pr.DeepCopy()
					prCopy.Finalizers = []string{}
					K8SClient.Update(testCtx, prCopy)
					// Now delete the resource
					K8SClient.Delete(testCtx, &pr)
				}
			}
		*/
	})

	BeforeEach(func() {
		for _, cr := range mainCRs {
			crCopy := cr.DeepCopyObject().(client.Object)
			err := K8SClient.Create(testCtx, crCopy)
			if err != nil && !errors.IsAlreadyExists(err) {
				Panic()
			}
		}

		for _, cr := range ctCRs {
			crCopy := cr.DeepCopyObject().(client.Object)
			err := K8SClient.Create(testCtx, crCopy)
			if err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).ToNot(HaveOccurred())
			}
		}

		Eventually(func() bool {
			newct := &provisioningv1alpha1.ClusterTemplate{}
			Expect(K8SClient.Get(context.Background(), client.ObjectKeyFromObject(ctComplete), newct)).To(Succeed())
			return newct.Status.Conditions != nil
		}, timeout, interval).Should(BeTrue())

		Eventually(func() bool {
			newct := &provisioningv1alpha1.ClusterTemplate{}
			Expect(K8SClient.Get(context.Background(), client.ObjectKeyFromObject(ctIncomplete), newct)).To(Succeed())
			return newct.Status.Conditions != nil
		}, timeout, interval).Should(BeTrue())
	})

	Context("Provisioning Request is created", func() {
		It("Verify status conditions if ClusterInstance rendering fails", func() {
			crName := "cluster-1"
			// Make sure the needed ClusterTemplate exists.
			oranCT := &provisioningv1alpha1.ClusterTemplate{}
			err := K8SClient.Get(testCtx, client.ObjectKeyFromObject(ctIncomplete), oranCT)
			Expect(err).ToNot(HaveOccurred())

			// Create the ProvisioningRequest.
			ProvRequestCR.Name = crName
			ProvRequestCR.Spec.TemplateParameters = runtime.RawExtension{
				Raw: []byte(testutils.TestFullTemplateParameters),
			}
			copyProvRequestCR := ProvRequestCR.DeepCopy()
			err = K8SClient.Create(testCtx, copyProvRequestCR)
			Expect(err).ToNot(HaveOccurred())

			reconciledPR := &provisioningv1alpha1.ProvisioningRequest{}
			Eventually(func() bool {
				err := K8SClient.Get(testCtx, client.ObjectKeyFromObject(ProvRequestCR), reconciledPR)
				Expect(err).ToNot(HaveOccurred())
				return len(reconciledPR.Status.Conditions) == 2
			}, time.Minute*1, time.Second*3).Should(BeTrue())

			conditions := reconciledPR.Status.Conditions
			// Verify the ProvisioningRequest's status conditions.
			Expect(len(conditions)).To(Equal(2))
			testutils.VerifyStatusCondition(conditions[0], metav1.Condition{
				Type:   string(provisioningv1alpha1.PRconditionTypes.Validated),
				Status: metav1.ConditionTrue,
				Reason: string(provisioningv1alpha1.CRconditionReasons.Completed),
			})
			testutils.VerifyStatusCondition(conditions[1], metav1.Condition{
				Type:    string(provisioningv1alpha1.PRconditionTypes.ClusterInstanceRendered),
				Status:  metav1.ConditionFalse,
				Reason:  string(provisioningv1alpha1.CRconditionReasons.Failed),
				Message: "ClusterInstance.siteconfig.open-cluster-management.io \"cluster-1\" is invalid: spec.nodes[0].templateRefs: Required value",
			})

			// Verify provisioningState is failed when the clusterInstance rendering fails.
			testutils.VerifyProvisioningStatus(reconciledPR.Status.ProvisioningStatus,
				provisioningv1alpha1.StateFailed, "Failed to render and validate ClusterInstance", nil)
		})
	})

	Context("When NodeAllocationRequest has been created", func() {

		It("Skips re-rendering the ClusterInstance when configuration changes occur during active provisioning", func() {
			crName := "cluster-2"
			// Make sure the needed ClusterTemplate exists.
			oranCT := &provisioningv1alpha1.ClusterTemplate{}
			err := K8SClient.Get(testCtx, client.ObjectKeyFromObject(ctComplete), oranCT)
			Expect(err).ToNot(HaveOccurred())

			// Create the ProvisioningRequest.
			ProvRequestCR.Name = crName
			templateParms := strings.Replace(
				testutils.TestFullTemplateParameters, "\"clusterName\": \"cluster-1\"", "\"clusterName\": \"cluster-2\"", 1)
			templateParms = strings.Replace(
				templateParms, "\"nodeClusterName\": \"exampleCluster\"", "\"nodeClusterName\": \"cluster-2\"", 1)
			ProvRequestCR.Spec.TemplateParameters = runtime.RawExtension{Raw: []byte(templateParms)}
			copyProvRequestCR := ProvRequestCR.DeepCopy()
			copyProvRequestCR.Spec.TemplateVersion = tVersion2
			err = K8SClient.Create(testCtx, copyProvRequestCR)
			Expect(err).ToNot(HaveOccurred())

			// Wait for NodeAllocationRequest to be created and initially have hardware provisioning in progress
			reconciledPR := &provisioningv1alpha1.ProvisioningRequest{}
			Eventually(func() bool {
				err := K8SClient.Get(testCtx, client.ObjectKeyFromObject(ProvRequestCR), reconciledPR)
				Expect(err).ToNot(HaveOccurred())
				// Check that we have at least the basic conditions and hardware provisioning has started
				if len(reconciledPR.Status.Conditions) < 4 {
					return false
				}
				// Look for HardwareProvisioned condition with status False (in progress)
				for _, cond := range reconciledPR.Status.Conditions {
					if cond.Type == string(provisioningv1alpha1.PRconditionTypes.HardwareProvisioned) &&
						cond.Status == metav1.ConditionFalse {
						return true
					}
				}
				return false
			}, time.Minute*1, time.Second*3).Should(BeTrue())

			// Verify initial state - hardware provisioning in progress
			conditions := reconciledPR.Status.Conditions
			testutils.VerifyStatusCondition(conditions[0], metav1.Condition{
				Type:   string(provisioningv1alpha1.PRconditionTypes.Validated),
				Status: metav1.ConditionTrue,
				Reason: string(provisioningv1alpha1.CRconditionReasons.Completed),
			})
			testutils.VerifyStatusCondition(conditions[1], metav1.Condition{
				Type:   string(provisioningv1alpha1.PRconditionTypes.ClusterInstanceRendered),
				Status: metav1.ConditionTrue,
				Reason: string(provisioningv1alpha1.CRconditionReasons.Completed),
			})
			// Find and verify HardwareProvisioned condition
			var hwProvCondition *metav1.Condition
			for i := range conditions {
				if conditions[i].Type == string(provisioningv1alpha1.PRconditionTypes.HardwareProvisioned) {
					hwProvCondition = &conditions[i]
					break
				}
			}
			Expect(hwProvCondition).ToNot(BeNil())
			testutils.VerifyStatusCondition(*hwProvCondition, metav1.Condition{
				Type:    string(provisioningv1alpha1.PRconditionTypes.HardwareProvisioned),
				Status:  metav1.ConditionFalse,
				Reason:  string(provisioningv1alpha1.CRconditionReasons.InProgress),
				Message: "Hardware provisioning is in progress",
			})
			// Verify the provisioningState moves to progressing.
			testutils.VerifyProvisioningStatus(reconciledPR.Status.ProvisioningStatus,
				provisioningv1alpha1.StateProgressing, "Hardware provisioning is in progress", nil)

			// Now provision the NodeAllocationRequest to complete hardware provisioning
			currentNp := &pluginsv1alpha1.NodeAllocationRequest{}
			Expect(K8SClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: ctlrutils.UnitTestHwmgrNamespace}, currentNp)).To(Succeed())
			currentNp.Status.Conditions = []metav1.Condition{
				{
					Status:             metav1.ConditionTrue,
					Reason:             string(hwmgmtv1alpha1.Completed),
					Type:               string(hwmgmtv1alpha1.Provisioned),
					LastTransitionTime: metav1.Now(),
				},
			}
			Expect(K8SClient.Status().Update(ctx, currentNp)).To(Succeed())
			// Create the expected nodes.
			testutils.CreateNodeResources(ctx, K8SClient, currentNp.Name)

			// Simulate callback-triggered reconciliation by adding callback annotation to ProvisioningRequest
			// This is needed because hardware status updates now only occur during callback-triggered reconciliations
			// First, get the latest version to ensure we have the correct resourceVersion
			Expect(K8SClient.Get(ctx, client.ObjectKeyFromObject(ProvRequestCR), ProvRequestCR)).To(Succeed())
			if ProvRequestCR.Annotations == nil {
				ProvRequestCR.Annotations = make(map[string]string)
			}
			ProvRequestCR.Annotations[ctlrutils.CallbackReceivedAnnotation] = fmt.Sprintf("%d", time.Now().Unix())
			Expect(K8SClient.Update(ctx, ProvRequestCR)).To(Succeed())

			// Give the reconciler a moment to process the NodeAllocationRequest status update
			time.Sleep(2 * time.Second)

			// The ProvisioningRequest should complete hardware provisioning.
			Eventually(func() bool {
				err := K8SClient.Get(testCtx, client.ObjectKeyFromObject(ProvRequestCR), reconciledPR)
				Expect(err).ToNot(HaveOccurred())
				// Look for HardwareProvisioned condition with status True (completed)
				for _, cond := range reconciledPR.Status.Conditions {
					if cond.Type == string(provisioningv1alpha1.PRconditionTypes.HardwareProvisioned) &&
						cond.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, time.Minute*1, time.Second*3).Should(BeTrue())
			conditions = reconciledPR.Status.Conditions
			// Find and verify HardwareProvisioned condition is now completed
			hwProvCondition = nil
			for i := range conditions {
				if conditions[i].Type == string(provisioningv1alpha1.PRconditionTypes.HardwareProvisioned) {
					hwProvCondition = &conditions[i]
					break
				}
			}
			Expect(hwProvCondition).ToNot(BeNil())
			testutils.VerifyStatusCondition(*hwProvCondition, metav1.Condition{
				Type:   string(provisioningv1alpha1.PRconditionTypes.HardwareProvisioned),
				Status: metav1.ConditionTrue,
				Reason: string(provisioningv1alpha1.CRconditionReasons.Completed),
			})

			// Update the ProvisioningRequest to use a ClusterTemplate pointing to a ConfigMap that would attempt to
			// trigger re-rendering the ClusterInstance.
			Expect(K8SClient.Get(testCtx, client.ObjectKeyFromObject(ProvRequestCR), reconciledPR)).To(Succeed())
			reconciledPR.Spec.TemplateVersion = tVersion1
			Expect(K8SClient.Update(testCtx, reconciledPR)).To(Succeed())

			// With the HardwareProvisioned condition set to True, the ClusterInstance should be created.
			// We now expect to skip re-rendering the ClusterInstance and mitigate a possible dry-run failure.
			err = K8SClient.Get(testCtx, client.ObjectKeyFromObject(ProvRequestCR), reconciledPR)
			Expect(err).ToNot(HaveOccurred())
			conditions = reconciledPR.Status.Conditions
			testutils.VerifyStatusCondition(conditions[1], metav1.Condition{
				Type:    string(provisioningv1alpha1.PRconditionTypes.ClusterInstanceRendered),
				Status:  metav1.ConditionTrue,
				Reason:  string(provisioningv1alpha1.CRconditionReasons.Completed),
				Message: "ClusterInstance rendered and passed dry-run validation",
			})

			// Find and verify HardwareProvisioned condition after template version change
			hwProvConditionAfterChange := &metav1.Condition{}
			found := false
			for i := range conditions {
				if conditions[i].Type == string(provisioningv1alpha1.PRconditionTypes.HardwareProvisioned) {
					hwProvConditionAfterChange = &conditions[i]
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "HardwareProvisioned condition should exist")
			testutils.VerifyStatusCondition(*hwProvConditionAfterChange, metav1.Condition{
				Type:   string(provisioningv1alpha1.PRconditionTypes.HardwareProvisioned),
				Status: metav1.ConditionTrue,
				Reason: string(provisioningv1alpha1.CRconditionReasons.Completed),
			})

			// Verify the provisioningState is progressing.
			testutils.VerifyProvisioningStatus(reconciledPR.Status.ProvisioningStatus,
				provisioningv1alpha1.StateProgressing, fmt.Sprintf("Waiting for ClusterInstance (%s) to be processed", crName), nil)
		})
	})
})

var _ = Describe("Metal3 Plugin E2E Test", func() {
	const metal3Timeout = time.Second * 120
	const metal3Interval = time.Second * 5

	var (
		metal3TestCtx      context.Context
		metal3Namespace    = "metal3-system"
		testServerColour   = "blue"
		testServerType     = "test-server-type"
		testResourcePoolID = "test-pool-001"
		testHwProfile      = "profile-spr-dual-processor-128g"
		// ClusterTemplate configuration
		metal3TName        = "metal3-clustertemplate"
		metal3TVersion     = "v1.0.0"
		metal3CtNamespace  = "metal3-clustertemplate-v4-16"
		metal3CiDefaultsCm = "metal3-clusterinstance-defaults"
		metal3PtDefaultsCm = "metal3-policytemplate-defaults"
		metal3HwTemplate   = "metal3-hwtemplate"

		// Test cluster configuration
		testClusterName = "metal3-test-cluster"

		// BareMetalHost test data - 3 available hosts but only 1 will be selected
		testBMHs = []struct {
			name             string
			macAddress       string
			bmcAddress       string
			hostname         string
			ramMB            int32
			hwProfile        string
			colour           string
			storageSizeBytes metal3v1alpha1.Capacity
			isPreferred      bool // Only one host will match the strict criteria
		}{
			{
				name:             "bmh-1",
				macAddress:       "aa:bb:cc:dd:ee:01",
				bmcAddress:       "redfish://192.168.1.101/redfish/v1/Systems/1",
				hostname:         "server-node-1.example.com",
				ramMB:            65536, // 64GB - meets minimum but not preferred
				hwProfile:        testHwProfile,
				colour:           "red",
				storageSizeBytes: 500000000000,
				isPreferred:      false,
			},
			{
				name:             "bmh-2",
				macAddress:       "aa:bb:cc:dd:ee:02",
				bmcAddress:       "redfish://192.168.1.102/redfish/v1/Systems/1",
				hostname:         "server-node-2.example.com",
				ramMB:            131072, // 128GB - this one will be selected
				hwProfile:        testHwProfile,
				colour:           "blue",
				storageSizeBytes: 600000000000,
				isPreferred:      true,
			},
			{
				name:             "bmh-3",
				macAddress:       "aa:bb:cc:dd:ee:03",
				bmcAddress:       "redfish://192.168.1.103/redfish/v1/Systems/1",
				hostname:         "server-node-3.example.com",
				ramMB:            32768, // 32GB - below minimum requirements
				hwProfile:        "profile-spr-single-processor-32G",
				storageSizeBytes: 6000000000000,
				colour:           "green",
				isPreferred:      false,
			},
		}
	)

	metal3TestCtx = context.Background()

	// Helper function to create BareMetalHost
	createBareMetalHost := func(bmhData struct {
		name             string
		macAddress       string
		bmcAddress       string
		hostname         string
		ramMB            int32
		hwProfile        string
		colour           string
		storageSizeBytes metal3v1alpha1.Capacity
		isPreferred      bool
	}) *metal3v1alpha1.BareMetalHost {
		return &metal3v1alpha1.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhData.name,
				Namespace: metal3Namespace,
				Labels: map[string]string{
					"resourceselector.clcm.openshift.io/server-colour": bmhData.colour,
					"resources.clcm.openshift.io/resourcePoolId":       testResourcePoolID,
					"resourceselector.clcm.openshift.io/server-type":   testServerType,
				},
			},
			Spec: metal3v1alpha1.BareMetalHostSpec{
				Online: true,
				BMC: metal3v1alpha1.BMCDetails{
					Address:         bmhData.bmcAddress,
					CredentialsName: fmt.Sprintf("%s-bmc-secret", bmhData.name),
				},
				BootMACAddress: bmhData.macAddress,
			},
		}
	}

	// Helper function to create HardwareData CR
	createHardwareData := func(bmhName string, bmhData struct {
		name             string
		macAddress       string
		bmcAddress       string
		hostname         string
		ramMB            int32
		hwProfile        string
		colour           string
		storageSizeBytes metal3v1alpha1.Capacity
		isPreferred      bool
	}) *metal3v1alpha1.HardwareData {
		return &metal3v1alpha1.HardwareData{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmhName,
				Namespace: metal3Namespace,
			},
			Spec: metal3v1alpha1.HardwareDataSpec{
				HardwareDetails: &metal3v1alpha1.HardwareDetails{
					Hostname: bmhData.hostname,
					CPU: metal3v1alpha1.CPU{
						Arch: "x86_64",
					},
					RAMMebibytes: int(bmhData.ramMB),
					NIC: []metal3v1alpha1.NIC{
						{
							Name: "eno1",
							MAC:  bmhData.macAddress,
						},
						{
							Name: "eth0",
							MAC:  fmt.Sprintf("%s:01", bmhData.macAddress[:14]),
						},
						{
							Name: "eth1",
							MAC:  fmt.Sprintf("%s:02", bmhData.macAddress[:14]),
						},
					},
					Storage: []metal3v1alpha1.Storage{
						{
							Name:         "sda",
							SizeBytes:    bmhData.storageSizeBytes,
							Rotational:   false,
							Type:         "SSD",
							Model:        "Samsung SSD 980 PRO 1TB",
							SerialNumber: fmt.Sprintf("SN-%s", bmhData.name),
						},
					},
				},
			},
		}
	}

	// Helper function to create BMC secret
	createBMCSecret := func(bmhName string) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-bmc-secret", bmhName),
				Namespace: metal3Namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"username": []byte("admin"),
				"password": []byte("password123"),
			},
		}
	}

	// Test resources to be created/cleaned up
	var testResources []client.Object
	var clusterTemplateResources []client.Object
	var provisioningRequest *provisioningv1alpha1.ProvisioningRequest

	BeforeEach(func() {
		// Create Metal3 namespace if it doesn't exist
		testResources = []client.Object{
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: metal3Namespace,
				},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: metal3CtNamespace,
				},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ztp-" + metal3CtNamespace,
				},
			},
		}

		// Create BareMetalHosts and associated resources
		for _, bmhData := range testBMHs {
			bmh := createBareMetalHost(bmhData)
			hwData := createHardwareData(bmhData.name, bmhData)
			bmcSecret := createBMCSecret(bmhData.name)

			testResources = append(testResources, bmh, hwData, bmcSecret)
		}

		// Create cluster template resources
		clusterTemplateResources = []client.Object{
			// ClusterImageSet for e2e tests
			&hivev1.ClusterImageSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "4.15.0",
				},
				Spec: hivev1.ClusterImageSetSpec{
					ReleaseImage: "quay.io/openshift-release-dev/ocp-release:4.15.0-x86_64",
				},
			},
			// ConfigMap for ClusterInstance defaults
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      metal3CiDefaultsCm,
					Namespace: metal3CtNamespace,
				},
				Data: map[string]string{
					ctlrutils.ClusterInstallationTimeoutConfigKey: "30m",
					ctlrutils.ClusterInstanceTemplateDefaultsConfigmapKey: `
clusterImageSetNameRef: "4.15.0"
holdInstallation: false
cpuPartitioningMode: AllNodes
networkType: OVNKubernetes
pullSecretRef:
  name: "pull-secret"
templateRefs:
- name: "ai-cluster-templates-v1"
  namespace: "siteconfig-operator"
nodes:
- role: master
  automatedCleaningMode: disabled
  ironicInspect: ""
  bootMode: UEFI
  nodeNetwork:
    interfaces:
    - name: eno1
      label: bootable-interface
    - name: eth0
      label: base-interface
    - name: eth1
      label: data-interface
  templateRefs:
  - name: "ai-node-templates-v1"
    namespace: "siteconfig-operator"
`,
				},
			},
			// ConfigMap for PolicyTemplate defaults
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      metal3PtDefaultsCm,
					Namespace: metal3CtNamespace,
				},
				Data: map[string]string{
					ctlrutils.ClusterConfigurationTimeoutConfigKey: "5m",
					ctlrutils.PolicyTemplateDefaultsConfigmapKey: `
cpu-isolated: "2-31"
cpu-reserved: "0-1"
defaultHugepagesSize: "1G"`,
				},
			},
			// HardwareTemplate - configured to select only 1 node from 3 available
			&hwmgmtv1alpha1.HardwareTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      metal3HwTemplate,
					Namespace: ctlrutils.InventoryNamespace,
				},
				Spec: hwmgmtv1alpha1.HardwareTemplateSpec{
					HardwarePluginRef:           testHardwarePluginRef,
					BootInterfaceLabel:          "bootable-interface",
					HardwareProvisioningTimeout: "10m",
					NodeGroupData: []hwmgmtv1alpha1.NodeGroupData{
						{
							Name:           "single-node",
							Role:           "master",
							ResourcePoolId: testResourcePoolID,
							HwProfile:      testHwProfile,
							ResourceSelector: map[string]string{
								"resourceselector.clcm.openshift.io/server-colour": testServerColour,
								"resourceselector.clcm.openshift.io/server-type":   testServerType,
								"hardwaredata/cpu_arch":                            "x86_64",
								"hardwaredata/storage;sizeBytes>500000000000":      "present",
								"hardwaredata/ramMebibytes;gt":                     "65536",
							},
						},
					},
				},
			},
			// HardwareProfile for Metal3 test
			&hwmgmtv1alpha1.HardwareProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testHwProfile,
					Namespace: testHwMgrPluginNameSpace,
				},
				Spec: hwmgmtv1alpha1.HardwareProfileSpec{
					// Basic hardware profile spec - minimal firmware config to satisfy CRD validation
					BiosFirmware: hwmgmtv1alpha1.Firmware{
						Version: "test-bios-v1.0",
						URL:     "https://example.com/bios-firmware.bin",
					},
				},
			},
			// Pull secret
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pull-secret",
					Namespace: metal3CtNamespace,
				},
				Data: map[string][]byte{
					".dockerconfigjson": []byte(testutils.TestSecretDataStr),
				},
				Type: corev1.SecretTypeDockerConfigJson,
			},
		}

		// ClusterTemplate
		clusterTemplate := &provisioningv1alpha1.ClusterTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      provisioningcontrollers.GetClusterTemplateRefName(metal3TName, metal3TVersion),
				Namespace: metal3CtNamespace,
			},
			Spec: provisioningv1alpha1.ClusterTemplateSpec{
				Name:       metal3TName,
				Version:    metal3TVersion,
				Release:    "4.15.0",
				TemplateID: "550e8400-e29b-41d4-a716-446655440000",
				Templates: provisioningv1alpha1.Templates{
					ClusterInstanceDefaults: metal3CiDefaultsCm,
					PolicyTemplateDefaults:  metal3PtDefaultsCm,
					HwTemplate:              metal3HwTemplate,
				},
				TemplateParameterSchema: runtime.RawExtension{Raw: []byte(testutils.TestFullTemplateSchema)},
			},
		}
		clusterTemplateResources = append(clusterTemplateResources, clusterTemplate)

		// Create all test resources
		for _, resource := range testResources {
			Expect(K8SClient.Create(metal3TestCtx, resource)).To(Succeed())
		}

		// Update BMH status to make them Available for controller selection
		for _, bmhData := range testBMHs {
			bmh := &metal3v1alpha1.BareMetalHost{}
			Expect(K8SClient.Get(metal3TestCtx, types.NamespacedName{
				Name:      bmhData.name,
				Namespace: metal3Namespace,
			}, bmh)).To(Succeed())

			// Set the status with hardware details and Available state
			bmh.Status = metal3v1alpha1.BareMetalHostStatus{
				Provisioning: metal3v1alpha1.ProvisionStatus{
					State: metal3v1alpha1.StateAvailable,
				},
				HardwareDetails: &metal3v1alpha1.HardwareDetails{
					Hostname: bmhData.hostname,
					CPU: metal3v1alpha1.CPU{
						Arch: "x86_64",
					},
					RAMMebibytes: int(bmhData.ramMB),
					NIC: []metal3v1alpha1.NIC{
						{
							Name: "eno1",
							MAC:  bmhData.macAddress,
						},
						{
							Name: "eth0",
							MAC:  fmt.Sprintf("%s:01", bmhData.macAddress[:14]),
						},
						{
							Name: "eth1",
							MAC:  fmt.Sprintf("%s:02", bmhData.macAddress[:14]),
						},
					},
					Storage: []metal3v1alpha1.Storage{
						{
							Name:         "sda",
							SizeBytes:    1000000000000, // 1TB
							Rotational:   false,         // SSD
							Type:         "SSD",
							Model:        "Samsung SSD 980 PRO 1TB",
							SerialNumber: fmt.Sprintf("SN-%s", bmhData.name),
						},
					},
				},
			}
			Expect(K8SClient.Status().Update(metal3TestCtx, bmh)).To(Succeed())
		}

		for _, resource := range clusterTemplateResources {
			err := K8SClient.Create(metal3TestCtx, resource)
			if err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).ToNot(HaveOccurred())
			}
		}

		// Wait for ClusterTemplate to be ready
		Eventually(func() bool {
			ct := &provisioningv1alpha1.ClusterTemplate{}
			err := K8SClient.Get(metal3TestCtx, client.ObjectKeyFromObject(clusterTemplate), ct)
			if err != nil {
				return false
			}
			return len(ct.Status.Conditions) > 0
		}, metal3Timeout, metal3Interval).Should(BeTrue())
	})

	AfterEach(func() {
		// Clean up cluster template resources (excluding namespaces)
		for _, resource := range clusterTemplateResources {
			if _, ok := resource.(*corev1.Namespace); !ok {
				K8SClient.Delete(metal3TestCtx, resource)
			}
		}

		// Clean up test resources (excluding namespaces)
		for _, resource := range testResources {
			if _, ok := resource.(*corev1.Namespace); !ok {
				K8SClient.Delete(metal3TestCtx, resource)
			}
		}
	})

	Context("When provisioning a cluster with Metal3 plugin", func() {
		It("Should select only 1 BareMetalHost from 3 available hosts based on strict hardware criteria", func() {
			// Create ProvisioningRequest
			templateParams := strings.Replace(
				testutils.TestFullTemplateParameters, "\"clusterName\": \"cluster-1\"", fmt.Sprintf("\"clusterName\": \"%s\"", testClusterName), 1)
			templateParams = strings.Replace(
				templateParams, "\"nodeClusterName\": \"exampleCluster\"", fmt.Sprintf("\"nodeClusterName\": \"%s\"", testClusterName), 1)

			// Verify BMHs don't have allocation labels before creating ProvisioningRequest
			preBmhList := &metal3v1alpha1.BareMetalHostList{}
			Expect(K8SClient.List(metal3TestCtx, preBmhList, client.InNamespace(metal3Namespace))).To(Succeed())
			for _, bmh := range preBmhList.Items {
				Expect(bmh.Labels).ToNot(HaveKey("clcm.openshift.io/allocated"),
					fmt.Sprintf("BMH %s should not have allocated label before ProvisioningRequest creation", bmh.Name))
				Expect(bmh.Labels).ToNot(HaveKey("clcm.openshift.io/allocated-node"),
					fmt.Sprintf("BMH %s should not have allocated-node label before ProvisioningRequest creation", bmh.Name))
			}

			provisioningRequest = &provisioningv1alpha1.ProvisioningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       testClusterName,
					Finalizers: []string{provisioningv1alpha1.ProvisioningRequestFinalizer},
				},
				Spec: provisioningv1alpha1.ProvisioningRequestSpec{
					Name:               testClusterName,
					Description:        "E2E test for Metal3 plugin: select 1 from 3 available BareMetalHosts",
					TemplateName:       metal3TName,
					TemplateVersion:    metal3TVersion,
					TemplateParameters: runtime.RawExtension{Raw: []byte(templateParams)},
				},
			}

			Expect(K8SClient.Create(metal3TestCtx, provisioningRequest)).To(Succeed())

			// Verify all 3 BareMetalHosts exist in the cluster
			bmhList := &metal3v1alpha1.BareMetalHostList{}
			Expect(K8SClient.List(metal3TestCtx, bmhList, client.InNamespace(metal3Namespace))).To(Succeed())
			Expect(len(bmhList.Items)).To(Equal(3), "All 3 BareMetalHosts should be available in inventory")

			// Verify the characteristics of each BMH
			for i, bmh := range bmhList.Items {
				expectedBMH := testBMHs[i]
				Expect(bmh.Name).To(Equal(expectedBMH.name))
				// Check if hardware details are available before accessing them
				if bmh.Status.HardwareDetails != nil {
					Expect(bmh.Status.HardwareDetails.RAMMebibytes).To(Equal(int(expectedBMH.ramMB)))
				}
			}

			// Wait for ProvisioningRequest to progress through validation
			Eventually(func() bool {
				pr := &provisioningv1alpha1.ProvisioningRequest{}
				err := K8SClient.Get(metal3TestCtx, client.ObjectKeyFromObject(provisioningRequest), pr)
				if err != nil {
					return false
				}

				// Check for Validated condition
				for _, cond := range pr.Status.Conditions {
					if cond.Type == string(provisioningv1alpha1.PRconditionTypes.Validated) &&
						cond.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, metal3Timeout, metal3Interval).Should(BeTrue())

			// Wait for ClusterInstance to be rendered
			Eventually(func() bool {
				pr := &provisioningv1alpha1.ProvisioningRequest{}
				err := K8SClient.Get(metal3TestCtx, client.ObjectKeyFromObject(provisioningRequest), pr)
				if err != nil {
					return false
				}

				for _, cond := range pr.Status.Conditions {
					if cond.Type == string(provisioningv1alpha1.PRconditionTypes.ClusterInstanceRendered) &&
						cond.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, metal3Timeout, metal3Interval).Should(BeTrue())

			// Wait for HardwareTemplate to be rendered
			Eventually(func() bool {
				pr := &provisioningv1alpha1.ProvisioningRequest{}
				err := K8SClient.Get(metal3TestCtx, client.ObjectKeyFromObject(provisioningRequest), pr)
				if err != nil {
					return false
				}

				for _, cond := range pr.Status.Conditions {
					if cond.Type == string(provisioningv1alpha1.PRconditionTypes.HardwareTemplateRendered) &&
						cond.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, metal3Timeout, metal3Interval).Should(BeTrue())

			// Wait for NodeAllocationRequest to be created
			Eventually(func() bool {
				nar := &pluginsv1alpha1.NodeAllocationRequest{}
				err := K8SClient.Get(metal3TestCtx, types.NamespacedName{
					Name:      testClusterName,
					Namespace: testHwMgrPluginNameSpace,
				}, nar)
				return err == nil
			}, metal3Timeout, metal3Interval).Should(BeTrue())

			// Verify that NodeAllocationRequest was created with correct specifications
			nar := &pluginsv1alpha1.NodeAllocationRequest{}
			Expect(K8SClient.Get(metal3TestCtx, types.NamespacedName{
				Name:      testClusterName,
				Namespace: testHwMgrPluginNameSpace,
			}, nar)).To(Succeed())

			// Verify NodeAllocationRequest contains expected node groups (only 1 single-node group)
			Expect(len(nar.Spec.NodeGroup)).To(Equal(1)) // Only single-node group

			// Verify the single node group
			singleNodeGroup := nar.Spec.NodeGroup[0]
			Expect(singleNodeGroup.NodeGroupData.Name).To(Equal("single-node"))
			Expect(singleNodeGroup.Size).To(Equal(1))                                // Only 1 node selected from 3 available
			Expect(singleNodeGroup.NodeGroupData.HwProfile).To(Equal(testHwProfile)) // Only server-2 matches

			// Wait for Metal3 controllers to automatically create AllocatedNode resources
			Eventually(func() bool {
				allocatedNodes := &pluginsv1alpha1.AllocatedNodeList{}
				err := K8SClient.List(metal3TestCtx, allocatedNodes, client.InNamespace(testHwMgrPluginNameSpace))
				if err != nil {
					return false
				}
				// Expect exactly 1 allocated node to be created by the controller
				return len(allocatedNodes.Items) == 1 && allocatedNodes.Items[0].Spec.NodeAllocationRequest == testClusterName
			}, metal3Timeout, metal3Interval).Should(BeTrue())

			// Trigger callback-based reconciliation
			pr := &provisioningv1alpha1.ProvisioningRequest{}
			Expect(K8SClient.Get(metal3TestCtx, client.ObjectKeyFromObject(provisioningRequest), pr)).To(Succeed())
			if pr.Annotations == nil {
				pr.Annotations = make(map[string]string)
			}
			pr.Annotations[ctlrutils.CallbackReceivedAnnotation] = fmt.Sprintf("%d", time.Now().Unix())
			Expect(K8SClient.Update(metal3TestCtx, pr)).To(Succeed())

			// Wait for hardware provisioning to complete
			Eventually(func() bool {
				pr := &provisioningv1alpha1.ProvisioningRequest{}
				err := K8SClient.Get(metal3TestCtx, client.ObjectKeyFromObject(provisioningRequest), pr)
				if err != nil {
					return false
				}

				for _, cond := range pr.Status.Conditions {
					if cond.Type == string(provisioningv1alpha1.PRconditionTypes.HardwareProvisioned) &&
						cond.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, metal3Timeout, metal3Interval).Should(BeTrue())

			// Verify final ProvisioningRequest status
			finalPR := &provisioningv1alpha1.ProvisioningRequest{}
			Expect(K8SClient.Get(metal3TestCtx, client.ObjectKeyFromObject(provisioningRequest), finalPR)).To(Succeed())

			// Verify all expected conditions are present and successful
			conditionTypes := []string{
				string(provisioningv1alpha1.PRconditionTypes.Validated),
				string(provisioningv1alpha1.PRconditionTypes.ClusterInstanceRendered),
				string(provisioningv1alpha1.PRconditionTypes.HardwareTemplateRendered),
				string(provisioningv1alpha1.PRconditionTypes.HardwareProvisioned),
			}

			for _, condType := range conditionTypes {
				found := false
				for _, cond := range finalPR.Status.Conditions {
					if cond.Type == condType && cond.Status == metav1.ConditionTrue {
						found = true
						break
					}
				}
				Expect(found).To(BeTrue(), fmt.Sprintf("Expected condition %s to be True", condType))
			}

			// Verify that the provisioning state indicates progress
			Expect(finalPR.Status.ProvisioningStatus.ProvisioningPhase).To(Equal(provisioningv1alpha1.StateProgressing))

			// Verify NodeAllocationRequest reference is set
			Expect(finalPR.Status.Extensions.NodeAllocationRequestRef).ToNot(BeNil())
			Expect(finalPR.Status.Extensions.NodeAllocationRequestRef.NodeAllocationRequestID).To(Equal(testClusterName))

			// Verify that bmh-2 (the selected BMH) has the correct allocation labels
			selectedBMH := &metal3v1alpha1.BareMetalHost{}
			Expect(K8SClient.Get(metal3TestCtx, types.NamespacedName{
				Name:      "bmh-2",
				Namespace: metal3Namespace,
			}, selectedBMH)).To(Succeed())

			// Check allocation labels are present
			Expect(selectedBMH.Labels).To(HaveKey("clcm.openshift.io/allocated"))
			Expect(selectedBMH.Labels["clcm.openshift.io/allocated"]).To(Equal("true"))

			// Check allocated-node label points to the correct AllocatedNode
			Expect(selectedBMH.Labels).To(HaveKey("clcm.openshift.io/allocated-node"))
			allocatedNodeName := selectedBMH.Labels["clcm.openshift.io/allocated-node"]
			Expect(allocatedNodeName).ToNot(BeEmpty())

			// Verify the AllocatedNode actually exists
			allocatedNode := &pluginsv1alpha1.AllocatedNode{}
			Expect(K8SClient.Get(metal3TestCtx, types.NamespacedName{
				Name:      allocatedNodeName,
				Namespace: testHwMgrPluginNameSpace,
			}, allocatedNode)).To(Succeed())

			// Verify the AllocatedNode references the correct NodeAllocationRequest
			Expect(allocatedNode.Spec.NodeAllocationRequest).To(Equal(testClusterName))

			// Verify that bmh-1 and bmh-3 (non-selected BMHs) do NOT have allocation labels
			nonSelectedBMHs := []string{"bmh-1", "bmh-3"}
			for _, bmhName := range nonSelectedBMHs {
				nonSelectedBMH := &metal3v1alpha1.BareMetalHost{}
				Expect(K8SClient.Get(metal3TestCtx, types.NamespacedName{
					Name:      bmhName,
					Namespace: metal3Namespace,
				}, nonSelectedBMH)).To(Succeed())

				// Check that allocation labels are NOT present
				Expect(nonSelectedBMH.Labels).ToNot(HaveKey("clcm.openshift.io/allocated"),
					fmt.Sprintf("BMH %s should not have allocated label since it was not selected", bmhName))
				Expect(nonSelectedBMH.Labels).ToNot(HaveKey("clcm.openshift.io/allocated-node"),
					fmt.Sprintf("BMH %s should not have allocated-node label since it was not selected", bmhName))
			}

			// Demonstrate successful resource selection:
			// - 3 BareMetalHosts were available in inventory
			// - Only 1 BMH (test-bmh-server-2) matched the strict hardware criteria:
			//   * 128GB RAM (gte:100000 mebibytes) - server-1 has 64GB, server-3 has 32GB
			//   * 56 CPU threads (gte:50) - server-1 has 48, server-3 has 32
			//   * CPU model contains "6348" - only server-2 has Gold 6348
			//   * Hardware profile "profile-spr-dual-processor-128g" - only server-2 matches
			// - The Metal3 plugin successfully filtered and selected only the matching host
		})
	})
})

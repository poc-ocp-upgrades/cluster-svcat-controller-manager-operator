package operator

import (
	"fmt"
	"os"
	"strings"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	appsclientv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	coreclientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	operatorapiv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourcehash"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	"github.com/openshift/cluster-svcat-controller-manager-operator/pkg/operator/v311_00_assets"
	"github.com/openshift/cluster-svcat-controller-manager-operator/pkg/util"
)

func syncServiceCatalogControllerManager_v311_00_to_latest(c ServiceCatalogControllerManagerOperator, originalOperatorConfig *operatorapiv1.ServiceCatalogControllerManager) (bool, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	errors := []error{}
	var err error
	operatorConfig := originalOperatorConfig.DeepCopy()
	directResourceResults := resourceapply.ApplyDirectly(c.kubeClient, c.recorder, v311_00_assets.Asset, "v3.11.0/openshift-svcat-controller-manager/ns.yaml", "v3.11.0/openshift-svcat-controller-manager/crb-catalog-controller.yaml", "v3.11.0/openshift-svcat-controller-manager/crb-controller-namespace-viewer-binding.yaml", "v3.11.0/openshift-svcat-controller-manager/cr-catalog-controller.yaml", "v3.11.0/openshift-svcat-controller-manager/rolebinding-cluster-info-configmap.yaml", "v3.11.0/openshift-svcat-controller-manager/rolebinding-configmap-accessor.yaml", "v3.11.0/openshift-svcat-controller-manager/role-cluster-info-configmap.yaml", "v3.11.0/openshift-svcat-controller-manager/role-configmap-accessor.yaml", "v3.11.0/openshift-svcat-controller-manager/sa.yaml", "v3.11.0/openshift-svcat-controller-manager/svc.yaml", "v3.11.0/openshift-svcat-controller-manager/servicemonitor-role.yaml", "v3.11.0/openshift-svcat-controller-manager/servicemonitor-rolebinding.yaml")
	resourcesThatForceRedeployment := sets.NewString("v3.11.0/openshift-svcat-controller-manager/sa.yaml")
	forceRollout := false
	for _, currResult := range directResourceResults {
		if currResult.Error != nil {
			errors = append(errors, fmt.Errorf("%q (%T): %v", currResult.File, currResult.Type, currResult.Error))
			continue
		}
		if currResult.Changed && resourcesThatForceRedeployment.Has(currResult.File) {
			forceRollout = true
		}
	}
	_, err = resourceapply.ApplyServiceMonitor(c.dynamicClient, c.recorder, v311_00_assets.MustAsset("v3.11.0/openshift-svcat-controller-manager/servicemonitor.yaml"))
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "servicemonitor", err))
	}
	_, configMapModified, err := manageServiceCatalogControllerManagerConfigMap_v311_00_to_latest(c.kubeClient, c.kubeClient.CoreV1(), c.recorder, operatorConfig)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "configmap", err))
	}
	clientCAModified, err := manageServiceCatalogControllerManagerClientCA_v311_00_to_latest(c.kubeClient.CoreV1(), c.recorder)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "client-ca", err))
	}
	forceRollout = forceRollout || operatorConfig.ObjectMeta.Generation != operatorConfig.Status.ObservedGeneration
	forceRollout = forceRollout || configMapModified || clientCAModified
	actualDaemonSet, _, err := manageServiceCatalogControllerManagerDeployment_v311_00_to_latest(c.kubeClient.AppsV1(), c.recorder, operatorConfig, c.targetImagePullSpec, operatorConfig.Status.Generations, forceRollout)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q: %v", "deployment", err))
	}
	operatorConfig.Status.ObservedGeneration = operatorConfig.ObjectMeta.Generation
	resourcemerge.SetDaemonSetGeneration(&operatorConfig.Status.Generations, actualDaemonSet)
	if actualDaemonSet.Status.NumberAvailable > 0 {
		v1helpers.SetOperatorCondition(&operatorConfig.Status.Conditions, operatorapiv1.OperatorCondition{Type: operatorapiv1.OperatorStatusTypeAvailable, Status: operatorapiv1.ConditionTrue})
	} else {
		v1helpers.SetOperatorCondition(&operatorConfig.Status.Conditions, operatorapiv1.OperatorCondition{Type: operatorapiv1.OperatorStatusTypeAvailable, Status: operatorapiv1.ConditionFalse, Reason: "NoPodsAvailable", Message: "no daemon pods available on any node."})
	}
	if actualDaemonSet.Status.NumberAvailable > 0 && actualDaemonSet.Status.UpdatedNumberScheduled == actualDaemonSet.Status.CurrentNumberScheduled {
		if len(actualDaemonSet.Annotations[util.VersionAnnotation]) > 0 {
			operatorConfig.Status.Version = actualDaemonSet.Annotations[util.VersionAnnotation]
		}
	}
	var progressingMessages []string
	if actualDaemonSet != nil && actualDaemonSet.ObjectMeta.Generation != actualDaemonSet.Status.ObservedGeneration {
		progressingMessages = append(progressingMessages, fmt.Sprintf("daemonset/controller-manager: observed generation is %d, desired generation is %d.", actualDaemonSet.Status.ObservedGeneration, actualDaemonSet.ObjectMeta.Generation))
	}
	if operatorConfig.ObjectMeta.Generation != operatorConfig.Status.ObservedGeneration {
		progressingMessages = append(progressingMessages, fmt.Sprintf("ServiceCatalogControllerManageroperatorconfigs/cluster: observed generation is %d, desired generation is %d.", operatorConfig.Status.ObservedGeneration, operatorConfig.ObjectMeta.Generation))
	}
	if len(progressingMessages) == 0 {
		v1helpers.SetOperatorCondition(&operatorConfig.Status.Conditions, operatorapiv1.OperatorCondition{Type: operatorapiv1.OperatorStatusTypeProgressing, Status: operatorapiv1.ConditionFalse})
	} else {
		v1helpers.SetOperatorCondition(&operatorConfig.Status.Conditions, operatorapiv1.OperatorCondition{Type: operatorapiv1.OperatorStatusTypeProgressing, Status: operatorapiv1.ConditionTrue, Reason: "DesiredStateNotYetAchieved", Message: strings.Join(progressingMessages, "\n")})
	}
	if len(errors) > 0 {
		message := ""
		for _, err := range errors {
			message = message + err.Error() + "\n"
		}
		v1helpers.SetOperatorCondition(&operatorConfig.Status.Conditions, operatorapiv1.OperatorCondition{Type: workloadFailingCondition, Status: operatorapiv1.ConditionTrue, Message: message, Reason: "SyncError"})
	} else {
		v1helpers.SetOperatorCondition(&operatorConfig.Status.Conditions, operatorapiv1.OperatorCondition{Type: workloadFailingCondition, Status: operatorapiv1.ConditionFalse})
	}
	if !equality.Semantic.DeepEqual(operatorConfig.Status, originalOperatorConfig.Status) {
		if _, err := c.operatorConfigClient.ServiceCatalogControllerManagers().UpdateStatus(operatorConfig); err != nil {
			return false, err
		}
	}
	if len(errors) > 0 {
		return true, nil
	}
	return false, nil
}
func manageServiceCatalogControllerManagerClientCA_v311_00_to_latest(client coreclientv1.CoreV1Interface, recorder events.Recorder) (bool, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	const apiserverClientCA = "client-ca"
	_, caChanged, err := resourceapply.SyncConfigMap(client, recorder, kubeAPIServerNamespaceName, apiserverClientCA, targetNamespaceName, apiserverClientCA, []metav1.OwnerReference{})
	if err != nil {
		return false, err
	}
	return caChanged, nil
}
func manageServiceCatalogControllerManagerConfigMap_v311_00_to_latest(kubeClient kubernetes.Interface, client coreclientv1.ConfigMapsGetter, recorder events.Recorder, operatorConfig *operatorapiv1.ServiceCatalogControllerManager) (*corev1.ConfigMap, bool, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	configMap := resourceread.ReadConfigMapV1OrDie(v311_00_assets.MustAsset("v3.11.0/openshift-svcat-controller-manager/cm.yaml"))
	defaultConfig := v311_00_assets.MustAsset("v3.11.0/openshift-svcat-controller-manager/defaultconfig.yaml")
	requiredConfigMap, _, err := resourcemerge.MergeConfigMap(configMap, "config.yaml", nil, defaultConfig, operatorConfig.Spec.UnsupportedConfigOverrides.Raw, operatorConfig.Spec.ObservedConfig.Raw)
	if err != nil {
		return nil, false, err
	}
	inputHashes, err := resourcehash.MultipleObjectHashStringMapForObjectReferences(kubeClient, resourcehash.NewObjectRef().ForConfigMap().InNamespace(targetNamespaceName).Named("client-ca"), resourcehash.NewObjectRef().ForSecret().InNamespace(targetNamespaceName).Named("serving-cert"))
	if err != nil {
		return nil, false, err
	}
	for k, v := range inputHashes {
		requiredConfigMap.Data[k] = v
	}
	return resourceapply.ApplyConfigMap(client, recorder, requiredConfigMap)
}
func manageServiceCatalogControllerManagerDeployment_v311_00_to_latest(client appsclientv1.DaemonSetsGetter, recorder events.Recorder, options *operatorapiv1.ServiceCatalogControllerManager, imagePullSpec string, generationStatus []operatorapiv1.GenerationStatus, forceRollout bool) (*appsv1.DaemonSet, bool, error) {
	_logClusterCodePath()
	defer _logClusterCodePath()
	required := resourceread.ReadDaemonSetV1OrDie(v311_00_assets.MustAsset("v3.11.0/openshift-svcat-controller-manager/ds.yaml"))
	if len(imagePullSpec) > 0 {
		required.Spec.Template.Spec.Containers[0].Image = imagePullSpec
	}
	level := 3
	switch options.Spec.LogLevel {
	case operatorapiv1.TraceAll:
		level = 8
	case operatorapiv1.Trace:
		level = 6
	case operatorapiv1.Debug:
		level = 4
	case operatorapiv1.Normal:
		level = 3
	}
	required.Spec.Template.Spec.Containers[0].Args = append(required.Spec.Template.Spec.Containers[0].Args, fmt.Sprintf("-v=%d", level))
	if required.Annotations == nil {
		required.Annotations = map[string]string{}
	}
	required.Annotations[util.VersionAnnotation] = os.Getenv("RELEASE_VERSION")
	return resourceapply.ApplyDaemonSet(client, recorder, required, resourcemerge.ExpectedDaemonSetGeneration(required, generationStatus), forceRollout)
}

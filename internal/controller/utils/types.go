package utils

// OranO2IMSConditionType defines conditions of an OranO2IMS deployment.
type OranO2IMSConditionType string

var OranO2IMSConditionTypes = struct {
	Ready                     OranO2IMSConditionType
	NotReady                  OranO2IMSConditionType
	Error                     OranO2IMSConditionType
	Available                 OranO2IMSConditionType
	MetadataServerAvailable   OranO2IMSConditionType
	DeploymentServerAvailable OranO2IMSConditionType
}{
	Ready:                     "ORANO2IMSReady",
	NotReady:                  "OranO2IMSConditionType",
	Error:                     "Error",
	Available:                 "Available",
	MetadataServerAvailable:   "MetadataServerAvailable",
	DeploymentServerAvailable: "DeploymentServerAvailable",
}

type OranO2IMSConditionReason string

var OranO2IMSConditionReasons = struct {
	DeploymentsReady                  OranO2IMSConditionReason
	DeploymentsError                  OranO2IMSConditionReason
	MetadataServerAvailable           OranO2IMSConditionReason
	DeploymentServerAvailable         OranO2IMSConditionReason
	ErrorGettingDeploymentInformation OranO2IMSConditionReason
	DeploymentNotFound                OranO2IMSConditionReason
}{
	DeploymentsReady:                  "DeploymensReady",
	DeploymentsError:                  "DeploymentsError",
	MetadataServerAvailable:           "MetadataServerAvailable",
	DeploymentServerAvailable:         "DeploymentServerAvailabe",
	ErrorGettingDeploymentInformation: "ErrorGettingDeploymentInformation",
	DeploymentNotFound:                "DeploymentNotFound",
}

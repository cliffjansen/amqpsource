/*
Copyright 2018 The Knative Authors

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

package v1alpha1

import (
	"github.com/knative/pkg/apis/duck"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Check that MqttSource can be validated and can be defaulted.
var _ runtime.Object = (*MqttSource)(nil)

// Check that MqttSource implements the Conditions duck type.
var _ = duck.VerifyType(&MqttSource{}, &duckv1alpha1.Conditions{})

// MqttSourceSpec defines the desired state of the source.
type MqttSourceSpec struct {

	// MQTT endpoint address and optional connection details.  If
	// connection details are provided, they override other provided
	// connection configuration.
	// Examples:
	//  myqueue
	//  mqtts://host:port/mytopic
	Address string `json:"address"`

	// Kubernetes secret containing default connection configuration
	// including password or TLS private key information.  Optional if
	// Address contains sufficient connection details.
	// +optional
	ConfigSecret corev1.SecretKeySelector `json:"configSecret,omitempty"`

	// Receiver credit window.  Number of in flight messages not yet
	// forwarded to the sink.  Default = 10.  Legal values: 1-10000.  Values
	// <= 0 are converted to the default.
	// +optional
	Credit int `json:"credit"`

	// ServiceAccountName is the name of the ServiceAccount to use to run this
	// source.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Sink is a reference to an object that will resolve to a domain name to use
	// as the sink.
	// +optional
	Sink *corev1.ObjectReference `json:"sink,omitempty"`
}

const (
	// MqttSourceConditionReady has status True when the
	// source is ready to send events.
	MqttSourceConditionReady = duckv1alpha1.ConditionReady

	// MqttSourceConditionSinkProvided has status True when the
	// MqttSource has been configured with a sink target.
	MqttSourceConditionSinkProvided duckv1alpha1.ConditionType = "SinkProvided"

	// MqttSourceConditionDeployed has status True when the
	// MqttSource has had it's receive adapter deployment created.
	MqttSourceConditionDeployed duckv1alpha1.ConditionType = "Deployed"
)

var mqttSourceCondSet = duckv1alpha1.NewLivingConditionSet(
	MqttSourceConditionSinkProvided,
	MqttSourceConditionDeployed)

// MqttSourceStatus defines the observed state of the source.
type MqttSourceStatus struct {
	// Conditions holds the state of a source at a point in time.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions duckv1alpha1.Conditions `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// SinkURI is the current active sink URI that has been configured for the source.
	// +optional
	SinkURI string `json:"sinkUri,omitempty"`
}

// GetCondition returns the condition currently associated with the given type, or nil.
func (s *MqttSourceStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return mqttSourceCondSet.Manage(s).GetCondition(t)
}

// IsReady returns true if the resource is ready overall.
func (s *MqttSourceStatus) IsReady() bool {
	return mqttSourceCondSet.Manage(s).IsHappy()
}

// InitializeConditions sets relevant unset conditions to Unknown state.
func (s *MqttSourceStatus) InitializeConditions() {
	mqttSourceCondSet.Manage(s).InitializeConditions()
}

// MarkSink sets the condition that the source has a sink configured.
func (s *MqttSourceStatus) MarkSink(uri string) {
	s.SinkURI = uri
	if len(uri) > 0 {
		condSet.Manage(s).MarkTrue(MqttSourceConditionSinkProvided)
	} else {
		condSet.Manage(s).MarkUnknown(MqttSourceConditionSinkProvided,
			"SinkEmpty", "Sink has resolved to empty.%s", "")
	}
}

// MarkNoSink sets the condition that the source does not have a sink configured.
func (s *MqttSourceStatus) MarkNoSink(reason, messageFormat string, messageA ...interface{}) {
	condSet.Manage(s).MarkFalse(MqttSourceConditionSinkProvided, reason, messageFormat, messageA...)
}

// MarkDeployed sets the condition that the source has been deployed.
func (s *MqttSourceStatus) MarkDeployed() {
	condSet.Manage(s).MarkTrue(MqttSourceConditionDeployed)
}

// MarkDeploying sets the condition that the source is deploying.
func (s *MqttSourceStatus) MarkDeploying(reason, messageFormat string, messageA ...interface{}) {
	condSet.Manage(s).MarkUnknown(MqttSourceConditionDeployed, reason, messageFormat, messageA...)
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MqttSource is the Schema for the mqttsources API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:categories=all,knative,eventing,sources
type MqttSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MqttSourceSpec   `json:"spec,omitempty"`
	Status MqttSourceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MqttSourceList contains a list of MqttSource
type MqttSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MqttSource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MqttSource{}, &MqttSourceList{})
}

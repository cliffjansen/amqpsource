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

// Check that AmqpSource can be validated and can be defaulted.
var _ runtime.Object = (*AmqpSource)(nil)

// Check that AmqpSource implements the Conditions duck type.
var _ = duck.VerifyType(&AmqpSource{}, &duckv1alpha1.Conditions{})

// AmqpSourceSpec defines the desired state of the source.
type AmqpSourceSpec struct {

	// AMQP endpoint address and optional connection details.  If
	// connection details are provided, they override other provided
	// connection configuration.
	// Examples:
	//  myqueue
	//  amqps://host:port/mytopic
	Address string `json:"address"`

	// Kubernetes secret containing default connection configuration
	// including password or TLS private key information.  Optional if
	// Address contains sufficient connection details. ZZZ format?
	ConfigSecret corev1.SecretKeySelector `json:"configSecret,omitempty"`

	// Receiver credit window.  Number of in flight messages not yet forwarded
	// to the sink.  Default = 10.  Legal values: 1-10000.
	// +optional
	Credit uint `json:"credit"`

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
	// AmqpSourceConditionReady has status True when the
	// source is ready to send events.
	AmqpSourceConditionReady = duckv1alpha1.ConditionReady

	// AmqpSourceConditionSinkProvided has status True when the
	// AmqpSource has been configured with a sink target.
	AmqpSourceConditionSinkProvided duckv1alpha1.ConditionType = "SinkProvided"

	// AmqpSourceConditionDeployed has status True when the
	// AmqpSource has had it's receive adapter deployment created.
	AmqpSourceConditionDeployed duckv1alpha1.ConditionType = "Deployed"
)

var amqpSourceCondSet = duckv1alpha1.NewLivingConditionSet(
	AmqpSourceConditionSinkProvided,
	AmqpSourceConditionDeployed)

// AmqpSourceStatus defines the observed state of the source.
type AmqpSourceStatus struct {
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
func (s *AmqpSourceStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return amqpSourceCondSet.Manage(s).GetCondition(t)
}

// IsReady returns true if the resource is ready overall.
func (s *AmqpSourceStatus) IsReady() bool {
	return amqpSourceCondSet.Manage(s).IsHappy()
}

// InitializeConditions sets relevant unset conditions to Unknown state.
func (s *AmqpSourceStatus) InitializeConditions() {
	amqpSourceCondSet.Manage(s).InitializeConditions()
}

// MarkSink sets the condition that the source has a sink configured.
func (s *AmqpSourceStatus) MarkSink(uri string) {
	s.SinkURI = uri
	if len(uri) > 0 {
		condSet.Manage(s).MarkTrue(AmqpSourceConditionSinkProvided)
	} else {
		condSet.Manage(s).MarkUnknown(AmqpSourceConditionSinkProvided,
			"SinkEmpty", "Sink has resolved to empty.%s", "")
	}
}

// MarkNoSink sets the condition that the source does not have a sink configured.
func (s *AmqpSourceStatus) MarkNoSink(reason, messageFormat string, messageA ...interface{}) {
	condSet.Manage(s).MarkFalse(AmqpSourceConditionSinkProvided, reason, messageFormat, messageA...)
}

// MarkDeployed sets the condition that the source has been deployed.
func (s *AmqpSourceStatus) MarkDeployed() {
	condSet.Manage(s).MarkTrue(AmqpSourceConditionDeployed)
}

// MarkDeploying sets the condition that the source is deploying.
func (s *AmqpSourceStatus) MarkDeploying(reason, messageFormat string, messageA ...interface{}) {
	condSet.Manage(s).MarkUnknown(AmqpSourceConditionDeployed, reason, messageFormat, messageA...)
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AmqpSource is the Schema for the amqpsources API
// +k8s:openapi-gen=true
// +kubebuilder:categories=all,knative,eventing,sources
type AmqpSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AmqpSourceSpec   `json:"spec,omitempty"`
	Status AmqpSourceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AmqpSourceList contains a list of AmqpSource
type AmqpSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AmqpSource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AmqpSource{}, &AmqpSourceList{})
}

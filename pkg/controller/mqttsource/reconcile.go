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

package mqttsource

import (
	"context"

	"go.uber.org/zap"

	"github.com/knative/eventing-sources/pkg/apis/sources/v1alpha1"
	"github.com/knative/eventing-sources/pkg/controller/mqttsource/resources"
	"github.com/knative/eventing-sources/pkg/controller/sinks"
	"github.com/knative/pkg/logging"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type reconciler struct {
	client              client.Client
	scheme              *runtime.Scheme
	recorder            record.EventRecorder
	receiveAdapterImage string
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two.
func (r *reconciler) Reconcile(ctx context.Context, object runtime.Object) (runtime.Object, error) {
	logger := logging.FromContext(ctx)

	source, ok := object.(*v1alpha1.MqttSource)
	if !ok {
		logger.Error("could not find MQTT source", zap.Any("object", object))
		return object, nil
	}

	// See if the source has been deleted
	accessor, err := meta.Accessor(source)
	if err != nil {
		logger.Warnf("Failed to get metadata accessor: %s", zap.Error(err))
		return object, err
	}
	// No need to reconcile if the source has been marked for deletion.
	deletionTimestamp := accessor.GetDeletionTimestamp()
	if deletionTimestamp != nil {
		return object, nil
	}

	source.Status.InitializeConditions()

	sinkURI, err := sinks.GetSinkURI(ctx, r.client, source.Spec.Sink, source.Namespace)
	if err != nil {
		source.Status.MarkNoSink("NotFound", "")
		return source, err
	}
	source.Status.MarkSink(sinkURI)

	args := &resources.AdapterArguments{
		Image:   r.receiveAdapterImage,
		Source:  source,
		Labels:  getLabels(source),
		SinkURI: sinkURI,
		Address: source.Spec.Address,
	}

	deploy, err := r.getDeployment(ctx, source)
	if err != nil {
		if errors.IsNotFound(err) {
			deploy, err = r.createDeployment(ctx, source, nil, args)
			if err != nil {
				r.recorder.Eventf(source, corev1.EventTypeNormal, "DeploymentBlocked", "waiting for %v", err)
				return object, err
			}
			r.recorder.Eventf(source, corev1.EventTypeNormal, "Deployed", "Created deployment %q", deploy.Name)
			source.Status.MarkDeploying("Deploying", "Created deployment %s", deploy.Name)
			// Since the Deployment has just been created, there's nothing more
			// to do until it gets a status. This MqttSource will be reconciled
			// again when the Deployment is updated.
			return object, nil
		}
		return object, err
	}

	// Update Deployment spec if it's changed
	expected := resources.MakeDeployment(nil, args)
	// Since the Deployment spec has fields defaulted by the webhook, it won't
	// be equal to expected. Use DeepDerivative to compare only the fields that
	// are set in expected.
	if !equality.Semantic.DeepDerivative(expected.Spec, deploy.Spec) {
		deploy.Spec = expected.Spec
		err := r.client.Update(ctx, deploy)
		// if no error, update the status.
		if err == nil {
			source.Status.MarkDeploying("DeployUpdated", "Updated deployment %s", deploy.Name)
		} else {
			source.Status.MarkDeploying("DeployNeedsUpdate", "Attempting to update deployment %s", deploy.Name)
			r.recorder.Eventf(source, corev1.EventTypeWarning, "DeployNeedsUpdate", "Failed to update deployment %q", deploy.Name)
		}
		// Return after this update or error and reconcile again
		return object, err
	}

	// Update source status
	if deploy.Status.ReadyReplicas > 0 {
		source.Status.MarkDeployed()
	}

	return source, nil
}

func (r *reconciler) getDeployment(ctx context.Context, source *v1alpha1.MqttSource) (*appsv1.Deployment, error) {
	logger := logging.FromContext(ctx)

	list := &appsv1.DeploymentList{}
	err := r.client.List(
		ctx,
		&client.ListOptions{
			Namespace:     source.Namespace,
			LabelSelector: labels.Everything(),
			// TODO this is here because the fake client needs it.
			// Remove this when it's no longer needed.
			Raw: &metav1.ListOptions{
				TypeMeta: metav1.TypeMeta{
					APIVersion: appsv1.SchemeGroupVersion.String(),
					Kind:       "Deployment",
				},
			},
		},
		list)
	if err != nil {
		logger.Errorf("Unable to list deployments: %v", zap.Error(err))
		return nil, err
	}
	for _, c := range list.Items {
		if metav1.IsControlledBy(&c, source) {
			return &c, nil
		}
	}
	return nil, errors.NewNotFound(schema.GroupResource{}, "")
}

func (r *reconciler) createDeployment(ctx context.Context, source *v1alpha1.MqttSource, org *appsv1.Deployment, args *resources.AdapterArguments) (*appsv1.Deployment, error) {
	deployment := resources.MakeDeployment(org, args)

	if err := controllerutil.SetControllerReference(source, deployment, r.scheme); err != nil {
		return nil, err
	}

	if err := r.client.Create(ctx, deployment); err != nil {
		return nil, err
	}
	return deployment, nil
}

func (r *reconciler) InjectClient(c client.Client) error {
	r.client = c
	return nil
}

func (r *reconciler) getLabelSelector(src *v1alpha1.MqttSource) labels.Selector {
	return labels.SelectorFromSet(getLabels(src))
}

func getLabels(src *v1alpha1.MqttSource) map[string]string {
	return map[string]string{
		"knative-eventing-source":      controllerAgentName,
		"knative-eventing-source-name": src.Name,
	}
}

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

package resources

import (
	"fmt"
	"strconv"

	"github.com/knative/eventing-sources/pkg/apis/sources/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AdapterArguments struct {
	Image   string
	Source  *v1alpha1.AmqpSource
	Labels  map[string]string
	SinkURI string
	Address string
	// TODO: full connect-config opts
}

const (
	credsVolume    = "amqp-config"
	credsMountPath = "/var/secrets/amqp"
	defaultCredit = 10
)


func MakeDeployment(org *appsv1.Deployment, args *AdapterArguments) *appsv1.Deployment {
	credit := args.Source.Spec.Credit
	if credit <= 0 {
		credit = defaultCredit
	}
	deploy := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("amqpsource-%s-", args.Source.Name),
			Namespace:    args.Source.Namespace,
			Labels:       args.Labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: args.Labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"sidecar.istio.io/inject": "true",
					},
					Labels: args.Labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: args.Source.Spec.ServiceAccountName,
					Containers: []corev1.Container{
						{
							Name:            "receive-adapter",
							Image:           args.Image,
							Env: []corev1.EnvVar{
								{
									Name:  "SINK_URI",
									Value: args.SinkURI,
								},
								{
									Name:  "AMQP_URI",
									Value: args.Address,
								},
								{
									Name:  "AMQP_CREDIT",
									Value: strconv.Itoa(credit),
								},
							},
							ImagePullPolicy: corev1.PullIfNotPresent,
						},
					},
				},
			},
		},
	}

	secretName := args.Source.Spec.ConfigSecret.Name
	if secretName != "" {
		mounts := []corev1.VolumeMount{  { Name:      credsVolume, MountPath: credsMountPath } }
		deploy.Spec.Template.Spec.Containers[0].VolumeMounts = mounts

		vols := []corev1.Volume{
			{
				Name: credsVolume,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: secretName,
					},
				},
			},
		}
		deploy.Spec.Template.Spec.Volumes = vols

		secretEnv := corev1.EnvVar{
			Name: "AMQP_CREDENTIALS",
			Value: credsMountPath,
		}
		deploy.Spec.Template.Spec.Containers[0].Env = append(deploy.Spec.Template.Spec.Containers[0].Env, secretEnv)
	}
	return deploy
}

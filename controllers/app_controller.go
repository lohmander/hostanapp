/*


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

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/iancoleman/strcase"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hostanv1alpha1 "github.com/lohmander/hostanapp/api/v1alpha1"
	provider "github.com/lohmander/hostanapp/provider"
	"github.com/lohmander/hostanapp/utils"
)

// AppReconciler reconciles a App object
type AppReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=hostan.hostan.app,resources=apps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hostan.hostan.app,resources=apps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

func getProviderWithName(providers *hostanv1alpha1.ProviderList, name string) (*hostanv1alpha1.Provider, error) {
	for _, provider := range providers.Items {
		if provider.ObjectMeta.Name == name {
			return &provider, nil
		}
	}

	return nil, fmt.Errorf("no provider found for %s", name)
}

// Reconcile implements the reconcile loop
func (r *AppReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("app", req.NamespacedName)

	app := &hostanv1alpha1.App{}
	err := r.Get(ctx, req.NamespacedName, app)

	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("App resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}

		log.Error(err, "Failed to get App")
		return ctrl.Result{}, err
	}

	result, err := r.reconcileProviders(log, app)

	// Return either if there's an error or we returned a
	// requeue result
	if err != nil || result.Requeue {
		return result, err
	}

	return r.reconcileAppServices(log, app)
}

func (r *AppReconciler) reconcileProviders(log logr.Logger, app *hostanv1alpha1.App) (ctrl.Result, error) {
	// Not all apps has uses
	if app.Spec.Uses == nil {
		return ctrl.Result{}, nil
	}

	var err error
	ctx := context.Background()
	providers := &hostanv1alpha1.ProviderList{}

	if err = r.List(ctx, providers); err != nil {
		log.Error(err, "Failed to get Providers")
		return ctrl.Result{}, err
	}

	// iterate through uses and get configuration from providers
	for _, use := range app.Spec.Uses {
		var configMap *corev1.ConfigMap

		provider_, err := getProviderWithName(providers, use.Name)

		if err != nil {
			return ctrl.Result{}, err
		}

		configMapName := fmt.Sprintf("%s-%s", app.Name, use.Name)
		found := &corev1.ConfigMap{}
		err = r.Get(ctx, types.NamespacedName{Namespace: app.Namespace, Name: configMapName}, found)

		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating config map", "name", configMapName)
			configMap, err := r.providerConfigMapForAppUse(app, provider_, &use)

			if err != nil {
				return ctrl.Result{}, err
			}

			err = r.Create(ctx, configMap)

			if err != nil {
				log.Error(err, "Failed to create config map", "name", configMapName)
				return ctrl.Result{}, err
			}

			log.Info("Successfully created config map", "name", configMapName)
			return ctrl.Result{Requeue: true}, nil
		}

		configHash, err := utils.CreateConfigHash(use.Config)

		if err != nil {
			return ctrl.Result{}, err
		}

		if found.Annotations["hostan.hostan.app/config-hash"] == *configHash {
			continue
		}

		configMap, err = r.providerConfigMapForAppUse(app, provider_, &use)

		if err != nil {
			return ctrl.Result{}, err
		}

		if err = r.Update(ctx, configMap); err != nil {
			if err != nil {
				return ctrl.Result{}, err
			}
		}

		log.Info("Successfully updated config map", "name", configMapName)
	}

	// Deprovision and delete old configs
	configMapList := &corev1.ConfigMapList{}
	err = r.List(
		ctx,
		configMapList,
		client.InNamespace(app.Namespace),
		client.MatchingLabels{"hostan.hostan.app/app": app.Name},
	)

	if err != nil {
		log.Error(err, "Failed to get config maps for app", "name", app.Name)
		return ctrl.Result{}, err
	}

	for _, cm := range configMapList.Items {
		if !utils.UsesProviderWithNameInApp(cm.Name, app) {
			log.Info("Deprovisining provider for configmap", "ConfigMap", cm.Name, "App", app.Name)

			provider_, err := getProviderWithName(providers, cm.Labels["hostan.hostan.app/provider"])

			if err != nil {
				return ctrl.Result{}, err
			}

			providerClient, err := provider.NewClient(provider_.Spec.URL)

			if err != nil {
				return ctrl.Result{}, err
			}

			_, err = providerClient.DeprovisionAppConfig(ctx, &provider.DeprovisionAppConfigRequest{
				AppName: app.Name,
			})

			if err != nil {
				return ctrl.Result{}, err
			}

			log.Info("Successfully deprovisioned", "App", app.Name, "WithProvider", cm.Labels["hostan.hostan.app/provider"])
			log.Info("Deleting configmap", "ConfigMap", cm.Name, "App", app.Name)
			err = r.Delete(ctx, &cm, client.GracePeriodSeconds(10))

			if err != nil {
				log.Error(err, "Failed to delete config map", cm.Name, "for app", app.Name)
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *AppReconciler) reconcileAppServices(log logr.Logger, app *hostanv1alpha1.App) (ctrl.Result, error) {
	var err error
	ctx := context.Background()

	providerConfigMaps := &corev1.ConfigMapList{}
	err = r.List(ctx, providerConfigMaps, client.MatchingLabels{"hostan.hostan.app/app": app.Name})

	if err != nil {
		return ctrl.Result{}, err
	}

	for _, service := range app.Spec.Services {
		// Reconcile deployments
		// deploy := r.deploymentForAppService(app, service, providerConfigMaps.Items)
		deploy := r.deploymentForAppService(app, service, []corev1.ConfigMap{})

		found := &appsv1.Deployment{}
		err = r.Get(ctx, types.NamespacedName{Name: deploy.Name, Namespace: app.Namespace}, found)

		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating new Deployment", "Namespace", deploy.Namespace, "Name", deploy.Name)

			err = r.Create(ctx, deploy)

			if err != nil {
				log.Error(err, "Failed to create new deployment for", deploy.Name)
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true}, nil
		}

		updated := false
		container := &found.Spec.Template.Spec.Containers[0] // For now we only support one container per pod

		// Check if the image changed, and if so update it
		if container.Image != service.Image {
			container.Image = service.Image
			updated = true
		}

		// Check if the service command changed, and if so update it
		if service.Command != nil && utils.StringSliceEquals(container.Command, service.Command) {
			container.Command = service.Command
			updated = true
		}

		// Check if the service port changed, and if so update it
		if container.Ports[0].ContainerPort != service.Port {
			container.Ports[0].ContainerPort = service.Port
			updated = true
		}

		// TODO: check if providers changed
		inConfigMapList := func(name string, ls *corev1.ConfigMapList) bool {
			for _, item := range ls.Items {
				if item.Name == name {
					return true
				}
			}

			return false
		}

		inEnvFromList := func(name string, ls []corev1.EnvFromSource) bool {
			for _, item := range ls {
				if item.ConfigMapRef.Name == name {
					return true
				}
			}

			return false
		}

		for _, envFrom := range container.EnvFrom {
			if !inConfigMapList(envFrom.ConfigMapRef.Name, providerConfigMaps) {
				updated = true
			}
		}

		for _, cm := range providerConfigMaps.Items {
			if !inEnvFromList(cm.Name, container.EnvFrom) {
				updated = true
			}
		}

		container.EnvFrom = deploy.Spec.Template.Spec.Containers[0].EnvFrom

		// If any property was updated, update the deployment and requeue
		if updated {
			err = r.Update(ctx, found)
			log.Info("Updated deployment for service", "App", app.Name, "Service", service.Name)

			if err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true}, nil
		}

		// Reconcile service
		_, err = r.reconcileService(log, app, service)

		if err != nil {
			log.Error(err, "Failed to reconcile Service", service.Name)
		}

		// Reconcile ingress
		_, err = r.reconcileIngress(log, app, service)

		if err != nil {
			log.Error(err, "Failed to reconcile Ingress", service.Name)
		}
	}

	// Delete orphaned resources
	deployList := &appsv1.DeploymentList{}
	err = r.List(
		ctx,
		deployList,
		client.InNamespace(app.Namespace),
		client.MatchingLabels{"app": app.Name},
	)

	if err != nil {
		log.Error(err, "Failed to get deployments for app")
		return ctrl.Result{}, err
	}

	for _, deploy := range deployList.Items {
		if !utils.ServiceWithNameInApp(deploy.Name, app) {
			log.Info("Deleting Deployment", "Deploy", deploy.Name, "App", app.Name)
			err = r.Delete(ctx, &deploy, client.GracePeriodSeconds(10))

			if err != nil {
				log.Error(err, "Failed to delete deployment", deploy.Name, "for app", app.Name)
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *AppReconciler) reconcileService(log logr.Logger, app *hostanv1alpha1.App, service hostanv1alpha1.AppService) (ctrl.Result, error) {
	ctx := context.Background()
	svc := r.serviceForAppService(app, service)
	found := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, found)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Created new (Kubernetes) Service", "Namespace", svc.Namespace, "Name", svc.Name)

		err = r.Create(ctx, svc)

		if err != nil {
			log.Error(err, "Failed to create new Service for", svc.Name)
			return ctrl.Result{}, nil
		}

		return ctrl.Result{Requeue: true}, nil
	}

	updated := false
	port := &found.Spec.Ports[0]

	if port.TargetPort.IntVal != service.Port {
		updated = true
		port.TargetPort = intstr.FromInt(int(service.Port))
		port.Port = service.Port
	}

	if updated {
		log.Info("Updating Service", "Name", svc.Name)
		err = r.Update(ctx, found)

		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Delete orphaned resources
	svcList := &corev1.ServiceList{}
	err = r.List(
		ctx,
		svcList,
		client.InNamespace(app.Namespace),
		client.MatchingLabels{"app": app.Name},
	)

	if err != nil {
		log.Error(err, "Failed to get svcments for app")
		return ctrl.Result{}, err
	}

	for _, svc := range svcList.Items {
		if !utils.ServiceWithNameInApp(svc.Name, app) {
			log.Info("Deleting (Kubernetes) Service", "Service", svc.Name, "App", app.Name)
			err = r.Delete(ctx, &svc, client.GracePeriodSeconds(10))

			if err != nil {
				log.Error(err, "Failed to delete (Kubernetes) Service", svc.Name, "for app", app.Name)
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *AppReconciler) reconcileIngress(log logr.Logger, app *hostanv1alpha1.App, service hostanv1alpha1.AppService) (ctrl.Result, error) {
	var err error
	ctx := context.Background()

	if service.Ingress != nil {
		ingress := r.ingressForAppService(app, service)
		found := &netv1.Ingress{}
		err = r.Get(ctx, types.NamespacedName{Name: ingress.Name, Namespace: ingress.Namespace}, found)

		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating new Ingress", "Namespace", ingress.Namespace, "Name", ingress.Name)

			err = r.Create(ctx, ingress)

			if err != nil {
				log.Error(err, "Failed to create new Ingress for", ingress.Name)
				return ctrl.Result{}, nil
			}

			return ctrl.Result{Requeue: true}, nil
		}

		updated := false
		rule := &found.Spec.Rules[0]

		if rule.Host != service.Ingress.Host {
			rule.Host = service.Ingress.Host
			updated = true
		}

		path := &rule.HTTP.Paths[0]
		serviceIngressPath := service.Ingress.Path

		// Default to root slash
		if serviceIngressPath == "" {
			serviceIngressPath = "/"
		}

		if path.Path != serviceIngressPath {
			path.Path = serviceIngressPath
			updated = true
		}

		if updated {
			log.Info("Updating Ingress", "Name", ingress.Name)
			err = r.Update(ctx, found)

			if err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The service has no ingress, but we still need to check if there is any previous one
		ingressName := nameForService(app, service)
		found := &netv1.Ingress{}
		err = r.Get(ctx, types.NamespacedName{Name: ingressName, Namespace: app.Namespace}, found)

		if err == nil || !errors.IsNotFound(err) {
			log.Info("Deleting Ingress", "Ingress", ingressName, "App", app.Name)
			err = nil
			err = r.Delete(ctx, found)

			if err != nil {
				log.Error(err, "Failed to delete Ingress")
				return ctrl.Result{}, nil
			}
		}
	}

	// Delete orphaned resources
	ingressList := &netv1.IngressList{}
	err = r.List(
		ctx,
		ingressList,
		client.InNamespace(app.Namespace),
		client.MatchingLabels{"app": app.Name},
	)

	if err != nil {
		log.Error(err, "Failed to get Ingresses for app")
		return ctrl.Result{}, err
	}

	for _, ing := range ingressList.Items {
		if !utils.ServiceWithNameInApp(ing.Name, app) {
			log.Info("Deleting Ingress", "Ingress", ing.Name, "App", app.Name)
			err = r.Delete(ctx, &ing, client.GracePeriodSeconds(10))

			if err != nil {
				log.Error(err, "Failed to delete Ingress", ing.Name, "for app", app.Name)
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *AppReconciler) deploymentForAppService(app *hostanv1alpha1.App, service hostanv1alpha1.AppService, providerConfigMaps []corev1.ConfigMap) *appsv1.Deployment {
	var replicas int32 = 1
	var envFrom []corev1.EnvFromSource

	for _, providerConfigMap := range providerConfigMaps {
		envFrom = append(envFrom, corev1.EnvFromSource{
			Prefix: fmt.Sprintf("%s_", strcase.ToScreamingSnake(providerConfigMap.Labels["hostan.hostan.app/provider"])),
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: providerConfigMap.Name,
				},
			},
		})
	}

	name := nameForService(app, service)
	labels := labelsForServiceDeployment(app, service)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: app.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    name,
						Image:   service.Image,
						Command: service.Command,
						Ports: []corev1.ContainerPort{{
							ContainerPort: service.Port,
						}},
						EnvFrom: envFrom,
					}},
				},
			},
		},
	}

	fmt.Println("app", app)
	fmt.Println("deploy", deploy)
	fmt.Println("scheme", r.Scheme)

	// Set the Hostan App instance as the owner and controller
	ctrl.SetControllerReference(
		app,
		deploy,
		r.Scheme,
	)

	return deploy
}

func (r *AppReconciler) serviceForAppService(app *hostanv1alpha1.App, service hostanv1alpha1.AppService) *corev1.Service {
	name := nameForService(app, service)
	labels := labelsForServiceDeployment(app, service)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: app.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:       name,
				Port:       service.Port,
				TargetPort: intstr.FromInt(int(service.Port)),
			}},
		},
	}

	// Set the Hostan App instance as the owner and controller
	ctrl.SetControllerReference(app, svc, r.Scheme)

	return svc
}

func (r *AppReconciler) ingressForAppService(app *hostanv1alpha1.App, service hostanv1alpha1.AppService) *netv1.Ingress {
	name := nameForService(app, service)
	labels := labelsForServiceDeployment(app, service)
	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: app.Namespace,
			Labels:    labels,
		},
		Spec: netv1.IngressSpec{
			Rules: []netv1.IngressRule{{
				Host: service.Ingress.Host,
				IngressRuleValue: netv1.IngressRuleValue{
					HTTP: &netv1.HTTPIngressRuleValue{
						Paths: []netv1.HTTPIngressPath{{
							Path: service.Ingress.Path,
							Backend: netv1.IngressBackend{
								ServiceName: name,
								ServicePort: intstr.FromInt(int(service.Port)),
							},
						}},
					},
				},
			}},
		},
	}

	// Set the Hostan App instance as the owner and controller
	ctrl.SetControllerReference(app, ingress, r.Scheme)

	return ingress
}

func (r *AppReconciler) providerConfigMapForAppUse(app *hostanv1alpha1.App, provider_ *hostanv1alpha1.Provider, use *hostanv1alpha1.AppUse) (*corev1.ConfigMap, error) {
	providerClient, err := provider.NewClient(provider_.Spec.URL)

	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	res, err := providerClient.ProvisionAppConfig(ctx, &provider.ProvisionAppConfigRequest{
		AppName: app.Name,
		Config:  use.Config,
	})

	if err != nil {
		return nil, err
	}

	configHash, err := utils.CreateConfigHash(use.Config)
	if err != nil {
		return nil, err
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", app.Name, provider_.Name),
			Namespace: app.Namespace,
			Annotations: map[string]string{
				"hostan.hostan.app/config-hash": *configHash,
			},
			Labels: map[string]string{
				"hostan.hostan.app/provider": use.Name,
				"hostan.hostan.app/app":      app.Name,
			},
		},
		Data: map[string]string{},
	}

	for _, kv := range res.ConfigVariables {
		configMap.Data[strcase.ToScreamingSnake(kv.Name)] = kv.Value
	}

	ctrl.SetControllerReference(app, configMap, r.Scheme)

	return configMap, nil
}

func nameForService(app *hostanv1alpha1.App, service hostanv1alpha1.AppService) string {
	return fmt.Sprintf("%s-%s", app.Name, service.Name)
}

func labelsForServiceDeployment(app *hostanv1alpha1.App, service hostanv1alpha1.AppService) map[string]string {
	return map[string]string{
		"operator": "hostanapp",
		"app":      app.Name,
		"service":  nameForService(app, service),
	}
}

func (r *AppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hostanv1alpha1.App{}).
		Complete(r)
}

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
	"github.com/lohmander/hostanapp/plan"
	"github.com/lohmander/hostanapp/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hostanv1 "github.com/lohmander/hostanapp/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	// LabelApp is used for the label placed on each k8s resource for referencing the App object
	LabelApp = "app.hostan.app/app"

	// LabelProvider is used to make configuration resources queryable by provider
	LabelProvider = "app.hostan.app/provider"

	// AnnotationConfigHash is used to compare whether or not the underlying configuration for a
	// given provisioned configuration resource has changed since it was provisioned
	AnnotationConfigHash = "app.hostan.app/config-hash"
)

// UseConfigName constructs the name string for a use config (ConfigMap & Secret)
func UseConfigName(app *hostanv1.App, use hostanv1.AppUse) string {
	return fmt.Sprintf("%s-%s", app.Name, use.Name)
}

// ServiceName constructs the name string for a service (Deployment & Service)
func ServiceName(app *hostanv1.App, service hostanv1.AppService) string {
	return fmt.Sprintf("%s-%s", app.Name, service.Name)
}

// AppReconciler reconciles a App object
type AppReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=hostan.hostan.app,resources=apps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hostan.hostan.app,resources=apps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile implements the reconciliation loop
func (r *AppReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	var err error

	ctx := context.Background()
	log := r.Log.WithValues("app", req.NamespacedName)

	// Get the App resource
	app := &hostanv1.App{}

	if err = r.Get(ctx, req.NamespacedName, app); err != nil {
		if errors.IsNotFound(err) {
			log.Info("App resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}

		log.Error(err, "Failed to get App")
		return ctrl.Result{}, err
	}

	current, err := r.AllCurrentObjects(req, app)
	desired, err := r.AllDesiredObjects(req, app)

	p, err := plan.Make(current, desired)

	fmt.Println(p.Describe())

	if err := p.Execute(); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// AllDesiredObjects returns a list of resources that are part of the "desired" state that we
// will attempt to reconcile towards
func (r *AppReconciler) AllDesiredObjects(req ctrl.Request, app *hostanv1.App) ([]plan.Resource, error) {
	var objects []plan.Resource

	for _, use := range app.Spec.Uses {
		objects = append(objects, &UseStateObject{r, use, app, nil, nil})
	}

	hasIngress := false

	for _, service := range app.Spec.Services {
		objects = append(objects, &ServiceStateObject{r, service, app, nil, nil, nil, nil})

		if service.Ingress != nil {
			hasIngress = true
		}
	}

	if hasIngress {
		objects = append(objects, &IngressStateObject{r, app, nil})
	}

	return objects, nil
}

func (r *AppReconciler) AllCurrentObjects(req ctrl.Request, app *hostanv1.App) ([]plan.Resource, error) {
	var objects []plan.Resource

	ctx := context.Background()
	labelsSelector := client.MatchingLabels{LabelApp: req.Name}
	namespace := client.InNamespace(req.Namespace)
	selectors := []client.ListOption{namespace, labelsSelector}

	configMapList := corev1.ConfigMapList{}

	if err := r.List(ctx, &configMapList, selectors...); err != nil {
		return nil, err
	}

	useMap := map[string]*UseStateObject{}

	toMapKey := func(labels map[string]string) string {
		return fmt.Sprintf("%s::%s", labels[LabelApp], labels[LabelProvider])
	}

	for _, x := range configMapList.Items {
		var appUse hostanv1.AppUse

		cm := x
		key := toMapKey(cm.Labels)
		provider := cm.Labels[LabelProvider]

		for _, use := range app.Spec.Uses {
			if use.Name == provider {
				appUse = use
				break
			}
		}

		useMap[key] = &UseStateObject{
			r, appUse, app, &cm, nil,
		}
	}

	secretList := corev1.SecretList{}

	if err := r.List(ctx, &secretList, selectors...); err != nil {
		return nil, err
	}

	for _, x := range secretList.Items {
		var appUse hostanv1.AppUse

		sec := x
		key := toMapKey(sec.Labels)
		provider := sec.Labels[LabelProvider]

		for _, use := range app.Spec.Uses {
			if use.Name == provider {
				appUse = use
				break
			}
		}

		if _, ok := useMap[key]; ok {
			useMap[key].Secret = &sec
		} else {
			useMap[key] = &UseStateObject{
				r, appUse, app, nil, &sec,
			}
		}
	}

	for _, use := range useMap {
		objects = append(objects, use)
	}

	serviceMap := map[string]*ServiceStateObject{}

	deployList := appsv1.DeploymentList{}

	if err := r.List(ctx, &deployList, selectors...); err != nil {
		return nil, err
	}

	for _, x := range deployList.Items {
		var appService *hostanv1.AppService = nil

		// We need to copy the deploy (x) or the pointer (of all service objects) will point
		// to the last deployment in deployList after the for loop completes
		deploy := x

		for _, service := range app.Spec.Services {
			if ServiceName(app, service) == deploy.Name {
				appService = &service
				break
			}
		}

		// If app service is removed, we won't find any, but we still need a temporary representation
		// in the ServiceStateObjects before the k8s resource is removed
		if appService == nil {
			appService = &hostanv1.AppService{}
		}

		serviceMap[deploy.Name] = &ServiceStateObject{
			r, *appService, app, &deploy, nil, configMapList.Items, secretList.Items,
		}
	}

	for _, service := range serviceMap {

		objects = append(objects, service)
	}

	ingressList := netv1.IngressList{}

	if err := r.List(ctx, &ingressList, selectors...); err != nil {
		return nil, err
	}

	if len(ingressList.Items) > 0 {
		objects = append(objects, &IngressStateObject{
			r, app, &ingressList.Items[0],
		})
	}

	return objects, nil

}

// Resource implementations

// Services

type ServiceStateObject struct {
	Reconciler *AppReconciler
	AppService hostanv1.AppService
	App        *hostanv1.App
	Deployment *appsv1.Deployment
	Service    *corev1.Service
	ConfigMaps []corev1.ConfigMap
	Secrets    []corev1.Secret
}

func (sso *ServiceStateObject) Changed() bool {
	if deploy := sso.Deployment; deploy != nil {
		if deploy.Spec.Template.Spec.Containers[0].Image != sso.AppService.Image {
			return true
		}

		if deploy.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort != sso.AppService.Port {
			return true
		}

		if !utils.StringSliceEquals(deploy.Spec.Template.Spec.Containers[0].Command, sso.AppService.Command) {
			return true
		}

		for _, use := range sso.App.Spec.Uses {
			var configMap *corev1.ConfigMap
			var secret *corev1.Secret

			if sso.ConfigMaps != nil {
				for _, cm := range sso.ConfigMaps {
					if cm.Name == UseConfigName(sso.App, use) {
						configMap = &cm
						break
					}
				}
			}

			if sso.Secrets != nil {
				for _, sec := range sso.Secrets {
					if sec.Name == UseConfigName(sso.App, use) {
						secret = &sec
						break
					}
				}
			}

			useState := UseStateObject{
				sso.Reconciler,
				use,
				sso.App,
				configMap,
				secret,
			}

			if useState.Changed() {
				return true
			}
		}
	}

	return false
}

func (sso *ServiceStateObject) ToString() string {
	return fmt.Sprintf("svc:%s", ServiceName(sso.App, sso.AppService))
}

func (sso *ServiceStateObject) Create() error {
	ctx := context.Background()
	labels := map[string]string{
		LabelApp: sso.App.Name,
	}
	meta := metav1.ObjectMeta{
		Name:      ServiceName(sso.App, sso.AppService),
		Namespace: sso.App.Namespace,
		Labels:    labels,
	}

	envFroms, err := sso.UseConfigEnvFrom()

	if err != nil {
		return err
	}

	var replicas int32 = 1

	deploy := appsv1.Deployment{
		ObjectMeta: meta,
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: meta,
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    sso.AppService.Name,
						Image:   sso.AppService.Image,
						Ports:   []corev1.ContainerPort{{ContainerPort: sso.AppService.Port}},
						EnvFrom: envFroms,
						Env: []corev1.EnvVar{{
							Name: "HOSTANAPP_TICK",
							// TODO: implement a real tick
							Value: "1",
						}},
					}},
				},
			},
		},
	}

	ctrl.SetControllerReference(sso.App, &deploy, sso.Reconciler.Scheme)

	err = sso.Reconciler.Create(ctx, &deploy)

	if err != nil {
		return err
	}

	service := corev1.Service{
		ObjectMeta: meta,
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       meta.Name,
				Protocol:   "TCP",
				Port:       sso.AppService.Port,
				TargetPort: intstr.FromInt(int(sso.AppService.Port)),
			}},
			Selector: labels,
		},
	}

	ctrl.SetControllerReference(sso.App, &service, sso.Reconciler.Scheme)

	return sso.Reconciler.Create(ctx, &service)
}

func (sso *ServiceStateObject) Update() error {
	envFroms, err := sso.UseConfigEnvFrom()

	if err != nil {
		return err
	}

	ctx := context.Background()
	container := &sso.Deployment.Spec.Template.Spec.Containers[0]
	container.Image = sso.AppService.Image
	container.Command = sso.AppService.Command
	container.Ports[0].ContainerPort = sso.AppService.Port
	container.EnvFrom = envFroms

	if err := sso.Reconciler.Update(ctx, sso.Deployment); err != nil {
		return err
	}

	return nil
}

func (sso *ServiceStateObject) Delete() error {
	ctx := context.Background()

	if sso.Deployment != nil {
		if err := sso.Reconciler.Delete(ctx, sso.Deployment); err != nil {
			return err
		}
	}

	if sso.Service != nil {
		if err := sso.Reconciler.Delete(ctx, sso.Service); err != nil {
			return err
		}
	}

	return nil

}

func (sso *ServiceStateObject) UseConfigEnvFrom() ([]corev1.EnvFromSource, error) {
	ctx := context.Background()
	labelsSelector := client.MatchingLabels{LabelApp: sso.App.Name}
	namespace := client.InNamespace(sso.App.Namespace)
	selectors := []client.ListOption{namespace, labelsSelector}

	configMapList := corev1.ConfigMapList{}

	if err := sso.Reconciler.List(ctx, &configMapList, selectors...); err != nil {
		return nil, err
	}

	secretList := corev1.SecretList{}

	if err := sso.Reconciler.List(ctx, &secretList, selectors...); err != nil {
		return nil, err
	}

	envFroms := []corev1.EnvFromSource{}

	for _, configMap := range configMapList.Items {
		envFroms = append(envFroms, corev1.EnvFromSource{
			Prefix: ConfigPrefix(configMap.Labels[LabelProvider]),
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMap.Name,
				},
			},
		})
	}

	for _, secret := range secretList.Items {
		envFroms = append(envFroms, corev1.EnvFromSource{
			Prefix: ConfigPrefix(secret.Labels[LabelProvider]),
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secret.Name,
				},
			},
		})
	}

	return envFroms, nil
}

func ConfigPrefix(provider string) string {

	return fmt.Sprintf("%s_", strcase.ToScreamingSnake(provider))
}

// Uses

type UseStateObject struct {
	Reconciler *AppReconciler
	Use        hostanv1.AppUse
	App        *hostanv1.App
	ConfigMap  *corev1.ConfigMap
	Secret     *corev1.Secret
}

func ProvisionFromProvider(reconciler *AppReconciler, provider string, app string, config map[string]string) (map[string]string, map[string]string, error) {
	ctx := context.Background()
	providers := hostanv1.ProviderList{}

	if err := reconciler.List(ctx, &providers); err != nil {
		return nil, nil, err
	}

	providerClientSet := &ProviderClientSet{providers}
	providerClient, err := providerClientSet.Get(provider)

	if err != nil {
		return nil, nil, err
	}

	return providerClient.Provision(app, config)
}

func (cm *UseStateObject) Changed() bool {
	configHash, err := utils.CreateConfigHash(cm.Use.Config)

	if err != nil {
		cm.Reconciler.Log.Error(err, "Failed to create config hash")
		return false
	}

	if cm.ConfigMap != nil {
		if cm.ConfigMap.Annotations[AnnotationConfigHash] != *configHash {
			return true
		}
	}

	if cm.Secret != nil {
		if cm.Secret.Annotations[AnnotationConfigHash] != *configHash {
			return true
		}
	}

	return false
}

func (cm *UseStateObject) ToString() string {
	return fmt.Sprintf("use:%s", UseConfigName(cm.App, cm.Use))
}

func (cm *UseStateObject) Create() error {
	ctx := context.Background()
	configData, secretData, err := ProvisionFromProvider(cm.Reconciler, cm.Use.Name, cm.App.Name, cm.Use.Config)

	if err != nil {
		return err
	}

	configHash, err := utils.CreateConfigHash(cm.Use.Config)

	if err != nil {
		return err
	}

	meta := metav1.ObjectMeta{
		Name:      UseConfigName(cm.App, cm.Use),
		Namespace: cm.App.Namespace,
		Labels: map[string]string{
			LabelApp:      cm.App.Name,
			LabelProvider: cm.Use.Name,
		},
		Annotations: map[string]string{
			AnnotationConfigHash: *configHash,
		},
	}
	configMap := corev1.ConfigMap{
		ObjectMeta: meta,
		Data:       configData,
	}

	ctrl.SetControllerReference(cm.App, &configMap, cm.Reconciler.Scheme)

	err = cm.Reconciler.Create(ctx, &configMap)

	if err != nil {
		return err
	}

	secret := corev1.Secret{
		ObjectMeta: meta,
		StringData: secretData,
	}

	ctrl.SetControllerReference(cm.App, &secret, cm.Reconciler.Scheme)

	return cm.Reconciler.Create(ctx, &secret)
}

func (cm *UseStateObject) Update() error {
	configData, secretData, err := ProvisionFromProvider(cm.Reconciler, cm.Use.Name, cm.App.Name, cm.Use.Config)

	if err != nil {
		return err
	}

	ctx := context.Background()

	if cm.ConfigMap != nil {
		cm.ConfigMap.Data = configData
		err = cm.Reconciler.Update(ctx, cm.ConfigMap)

		if err != nil {
			return err
		}
	}

	if cm.Secret != nil {
		cm.Secret.StringData = secretData
		err = cm.Reconciler.Update(ctx, cm.Secret)

		if err != nil {
			return err
		}
	}

	return nil
}

func (cm *UseStateObject) Delete() error {
	ctx := context.Background()

	if cm.Secret != nil {
		if err := cm.Reconciler.Delete(ctx, cm.Secret); err != nil {
			return err
		}
	}

	if cm.ConfigMap != nil {
		return cm.Reconciler.Delete(ctx, cm.ConfigMap)
	}

	return nil
}

// Ingresses

type IngressStateObject struct {
	Reconciler *AppReconciler
	App        *hostanv1.App
	Ingress    *netv1.Ingress
}

func (is *IngressStateObject) Changed() bool {
	if is.Ingress != nil {
		for i, service := range is.App.Spec.Services {
			if service.Ingress == nil {
				return true
			}

			if len(is.Ingress.Spec.Rules) > i {
				rule := is.Ingress.Spec.Rules[i]
				if rule.Host != service.Ingress.Host {
					return true
				}

				if rule.HTTP.Paths[0].Path != service.Ingress.Path {
					return true
				}
			}
		}
	}

	return false
}

func (is *IngressStateObject) ToString() string {
	return fmt.Sprintf("ing:%s", is.App.Name)
}

func MakeIngressRules(app *hostanv1.App, services []hostanv1.AppService) []netv1.IngressRule {
	rules := []netv1.IngressRule{}

	for _, service := range services {
		if service.Ingress != nil {
			rules = append(rules, netv1.IngressRule{
				Host: service.Ingress.Host,
				IngressRuleValue: netv1.IngressRuleValue{
					HTTP: &netv1.HTTPIngressRuleValue{
						Paths: []netv1.HTTPIngressPath{
							{
								Path: service.Ingress.Path,
								Backend: netv1.IngressBackend{
									ServiceName: ServiceName(app, service),
									ServicePort: intstr.FromInt(int(service.Port)),
								},
							},
						},
					},
				},
			})
		}
	}

	return rules
}

func (is *IngressStateObject) Create() error {
	ctx := context.Background()
	meta := metav1.ObjectMeta{
		Name:      is.App.Name,
		Namespace: is.App.Namespace,
		Labels: map[string]string{
			LabelApp: is.App.Name,
		},
	}

	ingress := netv1.Ingress{
		ObjectMeta: meta,
		Spec: netv1.IngressSpec{
			Rules: MakeIngressRules(is.App, is.App.Spec.Services),
		},
	}

	ctrl.SetControllerReference(is.App, &ingress, is.Reconciler.Scheme)

	return is.Reconciler.Create(ctx, &ingress)
}

func (is *IngressStateObject) Update() error {
	ctx := context.Background()
	ingress := is.Ingress
	ingress.Spec.Rules = MakeIngressRules(is.App, is.App.Spec.Services)

	if err := is.Reconciler.Update(ctx, is.Ingress); err != nil {
		return err
	}

	return nil
}
func (is *IngressStateObject) Delete() error {
	ctx := context.Background()
	return is.Reconciler.Delete(ctx, is.Ingress)
}

// SetupWithManager sets up the reconciler with a manager
func (r *AppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hostanv1.App{}).
		Complete(r)
}

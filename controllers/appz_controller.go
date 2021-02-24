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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hostanv1 "github.com/lohmander/hostanapp/api/v1"
	provider_grpc "github.com/lohmander/hostanapp/provider"
	"github.com/lohmander/hostanapp/utils"
)

type AppZReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=app.hostan.app,resources=apps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=app.hostan.app,resources=apps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

func (r *AppZReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
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

	// Get all providers
	providers := hostanv1.ProviderList{}

	if err = r.List(ctx, &providers); err != nil {
		log.Error(err, "Failed to get Providers")
		return ctrl.Result{}, err
	}

	providerClientSet := &ProviderClientSet{providers}

	// Find all managed ConfigMaps and Secrets
	labelsSelector := client.MatchingLabels{"app.hostan.app/app": req.Name}
	configMaps := &corev1.ConfigMapList{}
	secrets := &corev1.SecretList{}

	if err = r.List(ctx, configMaps, labelsSelector); err != nil {
		log.Error(err, "Failed to get ConfigMaps")
		return ctrl.Result{}, err
	}

	if err = r.List(ctx, secrets, labelsSelector); err != nil {
		log.Error(err, "Failed to get Secrets")
		return ctrl.Result{}, err
	}

	// Check which are still present in App.Uses
	changes := 0 // Count changes

	removalQueue := map[string][]interface{}{}

	for _, configMap := range configMaps.Items {
		providerName := configMap.Labels["app.hostan.app/provider"]

		// If it is still present in uses, skip
		if use := getUse(app.Spec.Uses, providerName); use != nil {
			continue
		}

		removalQueue[providerName] = append(removalQueue[providerName], &configMap)
	}

	for _, secret := range secrets.Items {
		providerName := secret.Labels["app.hostan.app/provider"]

		// If it is still present in uses, skip
		if use := getUse(app.Spec.Uses, providerName); use != nil {
			continue
		}

		removalQueue[providerName] = append(removalQueue[providerName], &secret)
	}

	for removedUse, items := range removalQueue {
		providerClient, err := providerClientSet.Get(removedUse)

		if err != nil {
			log.Error(err, "Failed to get provider client")
			return ctrl.Result{}, err
		}

		err = providerClient.Deprovision(req.Name)

		if err != nil {
			log.Error(err, "Failed to deprovision", "Name", req.Name)
			return ctrl.Result{}, err
		}

		for _, item := range items {
			err = r.Delete(ctx, item.(runtime.Object))
			if err != nil {
				log.Error(err, "Failed to delete stale ConfigMap/Secret")
				return ctrl.Result{}, err
			}
		}

	}

	// Iterate over uses
	for _, use := range app.Spec.Uses {
		providerClient, err := providerClientSet.Get(use.Name)

		if err != nil {
			log.Error(err, "Failed to get provider client for", "Name", use.Name)
			return ctrl.Result{}, err
		}

		configName := fmt.Sprintf("%s-%s", req.Name, use.Name)
		selectName := types.NamespacedName{Namespace: req.Namespace, Name: configName}

		// Check if present
		configMap := &corev1.ConfigMap{}

		if err = r.Get(ctx, selectName, configMap); err != nil && !errors.IsNotFound(err) {
			log.Error(err, "Failed to check for the presence of an existing ConfigMap")
			return ctrl.Result{}, err
		}

		secret := &corev1.Secret{}

		if err = r.Get(ctx, selectName, secret); err != nil && !errors.IsNotFound(err) {
			log.Error(err, "Failed to check for the presence of an existing Secret")
			return ctrl.Result{}, err
		}

		vars, secrets, err := providerClient.Provision(app.Name, use.Config)

		if err != nil {
			log.Error(err, "Failed to provision config for", "App name", app.Name, "Provider", use.Name)
			return ctrl.Result{}, err
		}

		useConfigHash, err := utils.CreateConfigHash(use.Config)

		if err != nil {
			log.Error(err, "Failed to create config hash")
			return ctrl.Result{}, err
		}

		configAnnotations := map[string]string{
			"app.hostan.app/config-hash": *useConfigHash,
		}
		configLabels := map[string]string{
			"app.hostan.app/app":      req.Name,
			"app.hostan.app/provider": use.Name,
		}

		if configMap.Name != "" {
			currentConfigHash := configMap.Annotations["app.hostan.app/config-hash"]

			// Skip if config hashes are identical
			if currentConfigHash == *useConfigHash {
				continue
			}

			if len(vars) > 0 {
				configMap.Annotations = configAnnotations
				configMap.Data = vars

				err = r.Update(ctx, configMap)

				if err != nil {
					log.Error(err, "Failed to update ConfigMap", "Name", configMap.Name)
					return ctrl.Result{}, err
				}
			}

			changes++
		} else {
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:        configName,
					Namespace:   req.Namespace,
					Annotations: configAnnotations,
					Labels:      configLabels,
				},
				Data: vars,
			}

			ctrl.SetControllerReference(app, configMap, r.Scheme)

			if err = r.Create(ctx, configMap); err != nil {
				log.Error(err, "Failed to create ConfigMap", "Name", configMap.Name)
				return ctrl.Result{}, err
			}
		}

		if secret.Name != "" {
			currentConfigHash := secret.Annotations["app.hostan.app/config-hash"]

			// Skip if config hashes are identical
			if currentConfigHash == *useConfigHash {
				continue
			}

			if len(vars) > 0 {
				secret.Annotations = configAnnotations
				secret.StringData = secrets

				err = r.Update(ctx, secret)

				if err != nil {
					log.Error(err, "Failed to update Secret", "Name", secret.Name)
					return ctrl.Result{}, err
				}
			}

			changes++
		} else {
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        configName,
					Namespace:   req.Namespace,
					Annotations: configAnnotations,
					Labels:      configLabels,
				},
				StringData: secrets,
			}

			ctrl.SetControllerReference(app, secret, r.Scheme)

			if err = r.Create(ctx, secret); err != nil {
				log.Error(err, "Failed to create Secret", "Name", secret.Name)
				return ctrl.Result{}, err
			}
		}
	}

	// Reconcile deployments
	deployList := &appsv1.DeploymentList{}

	if err = r.List(ctx, deployList, labelsSelector); err != nil {
		log.Error(err, "Failed to list deployments")
		return ctrl.Result{}, err
	}

	for _, deploy := range deployList.Items {
		if service := getService(app.Spec.Services, deploy.Labels[""]); service != nil {
			continue
		}

		if err = r.Delete(ctx, &deploy); err != nil {
			log.Error(err, "Failed to delete deployment", "Name", deploy.Name)
			return ctrl.Result{}, err
		}
	}

	for _, service := range app.Spec.Services {
		serviceName := fmt.Sprintf("%s-%s", app.Name, service.Name)

		deploy := &appsv1.Deployment{}
		deploySelectName := types.NamespacedName{Namespace: req.Namespace, Name: serviceName}

		if err = r.Get(ctx, deploySelectName, deploy); err != nil {
			if errors.IsNotFound(err) {

			} else {
				log.Error(err, "Failed to get deployment")
				return ctrl.Result{}, nil
			}
		} else {
			container := deploy.Spec.Template.Spec.Containers[0]
			container.Image = service.Image
			container.Command = service.Command
			container.Ports = []corev1.ContainerPort{{
				ContainerPort: service.Port,
			}}
		}
	}

	return ctrl.Result{}, nil
}

func (r *AppZReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hostanv1.App{}).
		Complete(r)
}

func getService(services []hostanv1.AppService, name string) *hostanv1.AppService {
	for _, service := range services {
		if service.Name == name {
			return &service
		}
	}

	return nil
}

func getUse(uses []hostanv1.AppUse, name string) *hostanv1.AppUse {
	for _, use := range uses {
		if use.Name == name {
			return &use
		}
	}

	return nil
}

type ProviderClient struct {
	hostanv1.Provider
}

func (client *ProviderClient) GetGRPCClient() (provider_grpc.ProviderClient, error) {
	return provider_grpc.NewClient(client.Spec.URL)
}

func (client *ProviderClient) Provision(appName string, config map[string]string) (map[string]string, map[string]string, error) {
	c, err := client.GetGRPCClient()

	if err != nil {
		return nil, nil, err
	}

	ctx := context.Background()
	res, err := c.ProvisionAppConfig(ctx, &provider_grpc.ProvisionAppConfigRequest{
		AppName: appName,
		Config:  config,
	})

	vars := map[string]string{}
	secrets := map[string]string{}

	for _, v := range res.ConfigVariables {
		if v.Secret {
			secrets[v.Name] = v.Value
		} else {
			vars[v.Name] = v.Value
		}
	}

	return vars, secrets, err
}

func (client *ProviderClient) Deprovision(appName string) error {
	c, err := client.GetGRPCClient()

	if err != nil {
		return err
	}

	ctx := context.Background()
	_, err = c.DeprovisionAppConfig(ctx, &provider_grpc.DeprovisionAppConfigRequest{
		AppName: appName,
	})

	return err
}

type ProviderClientSet struct {
	hostanv1.ProviderList
}

func (clientSet *ProviderClientSet) Get(name string) (*ProviderClient, error) {
	for _, provider := range clientSet.Items {
		if provider.Name == name {
			return &ProviderClient{Provider: provider}, nil
		}
	}

	return nil, fmt.Errorf("Could not find a provider with name %s", name)
}

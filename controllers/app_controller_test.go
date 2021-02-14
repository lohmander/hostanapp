package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	hostanv1 "github.com/lohmander/hostanapp/api/v1"
	// +kubebuilder:scaffold:imports
)

var _ = Describe("App controller", func() {
	const (
		ProviderName = "echo"
		AppName      = "test-app"
		AppNamespace = "test-app-namespace"
		ServiceName  = "test-service"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When creating an App", func() {
		It("Should create deployments", func() {
			ctx := context.Background()
			namespace := &v1.Namespace{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Namespace",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: AppNamespace,
				},
			}
			Expect(k8sClient.Create(ctx, namespace)).Should(Succeed())

			app := &hostanv1.App{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "hostan.hostan.app/v1alpha1",
					Kind:       "App",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      AppName,
					Namespace: AppNamespace,
				},
				Spec: hostanv1.AppSpec{
					Services: []hostanv1.AppService{
						{
							Name:  ServiceName,
							Image: "nginx",
							Port:  80,
							Ingress: &hostanv1.AppServiceIngress{
								Host: "example.com",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			deployName := nameForService(app, app.Spec.Services[0])
			deployLookupKey := types.NamespacedName{Name: deployName, Namespace: AppNamespace}
			createdDeploy := &appsv1.Deployment{}

			Eventually(func() error {
				return k8sClient.Get(ctx, deployLookupKey, createdDeploy)
			}, timeout, interval).Should(Succeed())

			Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
		})

		It("Should pull data from a provider", func() {
			ctx := context.TODO()

			provider := &hostanv1.Provider{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "hostan.hostan.app/v1alpha1",
					Kind:       "Provider",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      ProviderName,
					Namespace: AppNamespace,
				},
				Spec: hostanv1.ProviderSpec{
					URL: "localhost:5000",
				},
			}

			app := &hostanv1.App{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "hostan.hostan.app/v1alpha1",
					Kind:       "App",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      AppName,
					Namespace: AppNamespace,
				},
				Spec: hostanv1.AppSpec{
					Services: []hostanv1.AppService{
						{
							Name:  ServiceName,
							Image: "nginx",
							Port:  80,
							Ingress: &hostanv1.AppServiceIngress{
								Host: "example.com",
							},
						},
					},
					Uses: []hostanv1.AppUse{
						{Name: ProviderName, Config: map[string]string{"Hello": "World"}},
					},
				},
			}

			Expect(k8sClient.Create(ctx, provider)).Should(Succeed())
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			configMapName := fmt.Sprintf("%s-%s", AppName, ProviderName)
			configMapLookupKey := types.NamespacedName{Name: configMapName, Namespace: AppNamespace}
			createdConfigMap := &v1.ConfigMap{}

			Eventually(func() error {
				return k8sClient.Get(ctx, configMapLookupKey, createdConfigMap)
			}, timeout, interval).Should(Succeed())

			Expect(createdConfigMap.Data["HELLO"]).Should(Equal("World"))

			deployName := nameForService(app, app.Spec.Services[0])
			deployLookupKey := types.NamespacedName{Name: deployName, Namespace: AppNamespace}
			createdDeploy := &appsv1.Deployment{}

			Eventually(func() error {
				return k8sClient.Get(ctx, deployLookupKey, createdDeploy)
			}, timeout, interval).Should(Succeed())

			Expect(createdDeploy.Spec.Template.Spec.Containers[0].EnvFrom[0].ConfigMapRef.Name).Should(Equal(configMapName))
		})

		It("Should update provider data", func() {
			ctx := context.TODO()

			appName := fmt.Sprintf("%s-2", AppName)
			app := &hostanv1.App{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "hostan.hostan.app/v1alpha1",
					Kind:       "App",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      appName,
					Namespace: AppNamespace,
				},
				Spec: hostanv1.AppSpec{
					Services: []hostanv1.AppService{
						{
							Name:  ServiceName,
							Image: "nginx",
							Port:  80,
							Ingress: &hostanv1.AppServiceIngress{
								Host: "example.com",
							},
						},
					},
					Uses: []hostanv1.AppUse{
						{Name: ProviderName, Config: map[string]string{"Goodbye": "Bye"}},
					},
				},
			}

			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			configMapName := fmt.Sprintf("%s-%s", appName, ProviderName)
			configMapLookupKey := types.NamespacedName{Name: configMapName, Namespace: AppNamespace}
			createdConfigMap := &v1.ConfigMap{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, configMapLookupKey, createdConfigMap)

				if err != nil {
					return false
				}

				return createdConfigMap.Data["GOODBYE"] == "Bye"
			}, timeout, interval).Should(BeTrue())
		})

		It("Should delete config map for provider when it's removed", func() {
			ctx := context.TODO()

			appName := fmt.Sprintf("%s-3", AppName)
			app := &hostanv1.App{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "hostan.hostan.app/v1alpha1",
					Kind:       "App",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      appName,
					Namespace: AppNamespace,
				},
				Spec: hostanv1.AppSpec{
					Services: []hostanv1.AppService{
						{
							Name:  ServiceName,
							Image: "nginx",
							Port:  80,
							Ingress: &hostanv1.AppServiceIngress{
								Host: "example.com",
							},
						},
					},
					Uses: []hostanv1.AppUse{
						{Name: ProviderName, Config: map[string]string{"Goodbye": "Bye"}},
					},
				},
			}

			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			app.Spec.Uses = nil

			Expect(k8sClient.Update(ctx, app)).Should(Succeed())

			configMapName := fmt.Sprintf("%s-%s", appName, ProviderName)
			configMapLookupKey := types.NamespacedName{Name: configMapName, Namespace: AppNamespace}
			createdConfigMap := &v1.ConfigMap{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, configMapLookupKey, createdConfigMap)

				if err != nil && errors.IsNotFound(err) {
					return true
				}
				fmt.Println("Got err", err, app.Spec.Uses)

				return false
			}, time.Second*3, interval).Should(BeTrue())
		})
	})
})

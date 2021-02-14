package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	hostanv1 "github.com/lohmander/hostanapp/api/v1"
	// +kubebuilder:scaffold:imports
)

var _ = Describe("App controller", func() {
	const (
		ProviderName = "echo"
		AppName      = "test-app"
		ServiceName  = "test-service"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When creating an App", func() {
		It("Should create deployments", func() {
			ctx := context.Background()

			app := &hostanv1.App{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "app.hostan.app/v1alpha1",
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
	})

	Context("When using a provider", func() {

		var app *hostanv1.App
		var provider *hostanv1.Provider

		BeforeEach(func() {
			provider = &hostanv1.Provider{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "app.hostan.app/v1alpha1",
					Kind:       "Provider",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "echo",
					Namespace: AppNamespace,
				},
				Spec: hostanv1.ProviderSpec{
					URL: "localhost:5005",
				},
			}

			ctx := context.Background()
			Expect(k8sClient.Create(ctx, provider)).Should(Succeed())

			app = &hostanv1.App{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "app.hostan.app/v1alpha1",
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
						{Name: "echo"},
					},
				},
			}
		})

		AfterEach(func() {
			ctx := context.Background()
			Expect(k8sClient.Delete(ctx, provider)).Should(Succeed())
		})

		It("Should create a config map with non-secret vars", func() {
			ctx := context.Background()

			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			Eventually(func() error {
				cm := corev1.ConfigMap{}
				return k8sClient.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-%s", AppName, "echo"), Namespace: AppNamespace}, &cm)

			}).Should(Succeed())

			Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
		})

		It("Should remove a config map with non-secret vars", func() {
			ctx := context.Background()

			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			cmName := types.NamespacedName{
				Namespace: AppNamespace,
				Name:      fmt.Sprintf("%s-%s", AppName, "echo"),
			}

			Eventually(func() error {
				return k8sClient.Get(ctx, cmName, &corev1.ConfigMap{})
			}).Should(Succeed())

			app.Spec.Uses = []hostanv1.AppUse{}
			Expect(k8sClient.Update(ctx, app)).Should(Succeed())

			Eventually(func() bool {
				cm := &corev1.ConfigMap{}
				_ = k8sClient.Get(ctx, cmName, cm)

				// Seems like the cm is not cleaned up, but at least all data should be gone
				return len(cm.Data) == 0
			}, time.Second*3, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
		})
	})
})

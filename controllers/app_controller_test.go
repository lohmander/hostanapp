package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/types"

	hostanv1 "github.com/lohmander/hostanapp/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// +kubebuilder:scaffold:imports
)

var _ = Describe("App controller", func() {
	const (
		ProviderName   = "echo"
		AppName        = "test-app"
		AppServiceName = "test-service"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	var provider *hostanv1.Provider
	var app *hostanv1.App
	var i int = 1

	BeforeEach(func() {
		ctx := context.Background()
		provider = &hostanv1.Provider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "echo",
				Namespace: AppNamespace,
			},
			Spec: hostanv1.ProviderSpec{
				URL: "localhost:5005",
			},
		}

		Expect(k8sClient.Create(ctx, provider)).Should(Succeed())

		app = &hostanv1.App{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "app.hostan.app/v1alpha1",
				Kind:       "App",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%d", AppName, i),
				Namespace: AppNamespace,
			},
			Spec: hostanv1.AppSpec{
				Services: []hostanv1.AppService{
					{
						Name:  AppServiceName,
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

		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		i++
	})

	AfterEach(func() {
		ctx := context.Background()
		Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, provider)).Should(Succeed())
	})

	Context("Create a new app", func() {
		It("Should create config maps", func() {
			ctx := context.Background()

			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: UseConfigName(app, app.Spec.Uses[0])}, &corev1.ConfigMap{})
			}, timeout, interval).Should(Succeed())
		})

		It("Should create secrets", func() {
			ctx := context.Background()

			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: UseConfigName(app, app.Spec.Uses[0])}, &corev1.Secret{})
			}, timeout, interval).Should(Succeed())
		})

		It("Should create deployments", func() {
			ctx := context.Background()

			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: ServiceName(app, app.Spec.Services[0])}, &appsv1.Deployment{})
			}, timeout, interval).Should(Succeed())
		})

		It("Should create kubernetes services", func() {
			ctx := context.Background()

			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: ServiceName(app, app.Spec.Services[0])}, &corev1.Service{})
			}, timeout, interval).Should(Succeed())
		})

		It("Should create kubernetes ingress", func() {
			ctx := context.Background()

			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: app.Name}, &netv1.Ingress{})
			}, timeout, interval).Should(Succeed())
		})
	})
})

package controllers

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	hostanv1alpha1 "github.com/lohmander/hostanapp/api/v1alpha1"
	// +kubebuilder:scaffold:imports
)

var _ = Describe("App controller", func() {
	const (
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

			app := &hostanv1alpha1.App{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "hostan.hostan.app/v1alpha1",
					Kind:       "App",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      AppName,
					Namespace: AppNamespace,
				},
				Spec: hostanv1alpha1.AppSpec{
					Services: []hostanv1alpha1.AppService{
						{
							Name:  ServiceName,
							Image: "nginx",
							Port:  80,
							Ingress: &hostanv1alpha1.AppServiceIngress{
								Host: "example.com",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			appLookupKey := types.NamespacedName{Name: AppName, Namespace: AppNamespace}
			createdApp := &hostanv1alpha1.App{}

			Eventually(func() error {
				return k8sClient.Get(ctx, appLookupKey, createdApp)
			}, timeout, interval).Should(Succeed())

			deployName := nameForService(app, app.Spec.Services[0])
			deployLookupKey := types.NamespacedName{Name: deployName, Namespace: AppNamespace}
			createdDeploy := &appsv1.Deployment{}

			Eventually(func() error {
				return k8sClient.Get(ctx, deployLookupKey, createdDeploy)
			}, timeout, interval).Should(Succeed())
		})
	})
})

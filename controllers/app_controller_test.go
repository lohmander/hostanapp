package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	hostanv1 "github.com/lohmander/hostanapp/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1beta "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// +kubebuilder:scaffold:imports
)

func GetAppService(app *hostanv1.App, name string) *hostanv1.AppService {
	for _, service := range app.Spec.Services {
		if service.Name == name {
			return &service
		}
	}

	return nil
}

var _ = Describe("App controller", func() {
	const (
		ProviderName    = "echo"
		AppName         = "test-app"
		AppServiceName  = "test-service"
		AppServiceName2 = "another-test-service"
		AppCronJobName  = "test-job"

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
							Path: "/",
						},
					},
					{
						Name:  AppServiceName2,
						Image: "python",
						Port:  3000,
					},
				},
				CronJobs: []hostanv1.AppCronJob{
					{
						Name:     AppCronJobName,
						Image:    "alpine",
						Command:  []string{"echo", "'1'"},
						Schedule: "*/1 * * * *",
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

		It("Should set envFrom in deployments to provisioned config", func() {
			ctx := context.Background()
			configName := UseConfigName(app, app.Spec.Uses[0])

			Eventually(func() string {
				deploy := appsv1.Deployment{}

				if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: ServiceName(app, app.Spec.Services[0])}, &deploy); err != nil {
					return err.Error()
				}

				return deploy.Spec.Template.Spec.Containers[0].EnvFrom[0].ConfigMapRef.Name
			}, timeout, interval).Should(Equal(configName))
		})

		It("Should set envFrom in deployments to provisioned secret", func() {
			ctx := context.Background()
			configName := UseConfigName(app, app.Spec.Uses[0])

			Eventually(func() string {
				deploy := appsv1.Deployment{}

				if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: ServiceName(app, app.Spec.Services[0])}, &deploy); err != nil {
					return err.Error()
				}

				return deploy.Spec.Template.Spec.Containers[0].EnvFrom[1].SecretRef.Name
			}, timeout, interval).Should(Equal(configName))
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

		It("Should create a kubernetes cronjob", func() {
			ctx := context.Background()

			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: CronJobName(app, app.Spec.CronJobs[0])}, &batchv1beta.CronJob{})
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Update an app", func() {
		It("Should update a config map with new config", func() {
			ctx := context.Background()

			app.Spec.Uses[0].Config = map[string]string{
				"HELLO": "TEST",
			}

			Expect(k8sClient.Update(ctx, app)).Should(Succeed())

			Eventually(func() string {
				configMap := corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: UseConfigName(app, app.Spec.Uses[0])}, &configMap)

				if err != nil {
					return err.Error()
				}

				return configMap.Data["HELLO"]
			}, timeout, interval).Should(Equal("TEST"))
		})

		It("Should update a deployment with a new image", func() {
			ctx := context.Background()

			appService := GetAppService(app, AppServiceName)
			appService.Image = "node"
			app.Spec.Services[0].Image = "node"

			Expect(k8sClient.Update(ctx, app)).Should(Succeed())

			Eventually(func() string {
				deploy := appsv1.Deployment{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: ServiceName(app, *appService)}, &deploy)

				if err != nil {
					return err.Error()
				}

				return deploy.Spec.Template.Spec.Containers[0].Image
			}, timeout, interval).Should(Equal("node"))
		})

		It("Should update a deployment with a new port", func() {
			ctx := context.Background()

			appService := &app.Spec.Services[0]
			appService.Port = 5000
			app.Spec.Services[0].Port = 5000

			Expect(k8sClient.Update(ctx, app)).Should(Succeed())

			Eventually(func() int32 {
				deploy := appsv1.Deployment{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: ServiceName(app, *appService)}, &deploy)

				if err != nil {
					return 0
				}

				return deploy.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort
			}, timeout, interval).Should(Equal(int32(5000)))
		})

		It("Should update a deployment with a command", func() {
			ctx := context.Background()

			command := []string{"a", "b", "c"}
			appService := &app.Spec.Services[0]
			app.Spec.Services[0].Command = command

			Expect(k8sClient.Update(ctx, app)).Should(Succeed())

			Eventually(func() []string {
				deploy := appsv1.Deployment{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: ServiceName(app, *appService)}, &deploy)

				if err != nil {
					return []string{err.Error()}
				}

				return deploy.Spec.Template.Spec.Containers[0].Command
			}, timeout, interval).Should(BeEquivalentTo(command))
		})

		It("Should update an ingress with a host", func() {
			ctx := context.Background()

			host := "example2.com"
			app.Spec.Services[0].Ingress.Host = host

			Expect(k8sClient.Update(ctx, app)).Should(Succeed())

			Eventually(func() string {
				ingress := netv1.Ingress{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: app.Name}, &ingress)

				if err != nil {
					return err.Error()
				}

				return ingress.Spec.Rules[0].Host
			}, timeout, interval).Should(Equal(host))
		})

		It("Should update an ingress with a path", func() {
			ctx := context.Background()

			path := "/test"
			app.Spec.Services[0].Ingress.Path = path

			Expect(k8sClient.Update(ctx, app)).Should(Succeed())

			Eventually(func() string {
				ingress := netv1.Ingress{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: app.Name}, &ingress)

				if err != nil {
					return err.Error()
				}

				return ingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Path
			}, timeout, interval).Should(Equal(path))
		})

		It("Should delete an ingress if its not present in any app service", func() {
			ctx := context.Background()

			By("First check that it succeeds at getting the ingress")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: app.Name}, &netv1.Ingress{})
			}, timeout, interval).Should(Succeed())

			app.Spec.Services[0].Ingress = nil

			Expect(k8sClient.Update(ctx, app)).Should(Succeed())

			By("And then check that it is deleted")
			Eventually(func() bool {
				return errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: app.Name}, &netv1.Ingress{}))
			}, timeout, interval).Should(BeTrue())
		})

		It("Should delete a deployment if its not present as an app service", func() {
			ctx := context.Background()
			appService := app.Spec.Services[1]

			By("First check that it succeeds at getting the deployment")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: ServiceName(app, appService)}, &appsv1.Deployment{})
			}, timeout, interval).Should(Succeed())

			app.Spec.Services = []hostanv1.AppService{app.Spec.Services[0]}

			Expect(k8sClient.Update(ctx, app)).Should(Succeed())

			By("And then check that it is deleted")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: ServiceName(app, appService)}, &appsv1.Deployment{})
				// return errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: ServiceName(app, appService)}, &appsv1.Deployment{}))
			}, timeout, interval).ShouldNot(Succeed())
			// }, timeout, interval).Should(BeTrue())
		})

		It("Should delete an app use secret if the use is no longer used", func() {
			ctx := context.Background()

			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: UseConfigName(app, app.Spec.Uses[0])}, &corev1.Secret{})
			}, timeout, interval).Should(Succeed())

			use := app.Spec.Uses[0]
			app.Spec.Uses = []hostanv1.AppUse{}

			Expect(k8sClient.Update(ctx, app)).Should(Succeed())

			By("And then check that it is deleted")
			Eventually(func() bool {
				// return k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: UseConfigName(app, use)}, &corev1.Secret{})
				// return errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: ServiceName(app, appService)}, &appsv1.Deployment{}))
				return errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: UseConfigName(app, use)}, &corev1.Secret{}))
			}, timeout, interval).Should(BeTrue())
		})

		It("Should delete an app use configmap if the use is no longer used", func() {
			ctx := context.Background()

			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: UseConfigName(app, app.Spec.Uses[0])}, &corev1.ConfigMap{})
			}, timeout, interval).Should(Succeed())

			use := app.Spec.Uses[0]
			app.Spec.Uses = []hostanv1.AppUse{}

			Expect(k8sClient.Update(ctx, app)).Should(Succeed())

			By("And then check that it is deleted")
			Eventually(func() bool {
				// return k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: UseConfigName(app, use)}, &corev1.ConfigMap{})
				return errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: UseConfigName(app, use)}, &corev1.ConfigMap{}))
			}, timeout, interval).Should(BeTrue())
		})

		By("Adding an app use")
		It("Should mark any services as changed too", func() {
			ctx := context.Background()
			appService := app.Spec.Services[1]
			appUse := app.Spec.Uses[0]
			deploy := appsv1.Deployment{}

			By("First check that it succeeds at getting the deployment")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: ServiceName(app, appService)}, &deploy)
			}, timeout, interval).Should(Succeed())

			configMap := corev1.ConfigMap{}
			secret := corev1.Secret{}

			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: UseConfigName(app, appUse)}, &configMap); err != nil {
				Fail(err.Error())
			}

			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: UseConfigName(app, appUse)}, &secret); err != nil {
				Fail(err.Error())
			}

			// app.Spec.Uses = []hostanv1.AppUse{}

			sso := ServiceStateObject{ConfigEnvStateObject{nil, app}, nil, appService, app, &deploy, nil, []corev1.ConfigMap{configMap}, []corev1.Secret{secret}}

			Expect(sso.Changed()).To(BeFalse())

			app.Spec.Uses = append(app.Spec.Uses, hostanv1.AppUse{
				Name:   ProviderName,
				Config: map[string]string{},
			})

			sso = ServiceStateObject{ConfigEnvStateObject{nil, app}, nil, appService, app, &deploy, nil, []corev1.ConfigMap{configMap}, []corev1.Secret{secret}}

			Expect(sso.Changed()).To(BeTrue())
		})

		By("Changing an app use (through config, add, or delete)")
		It("Should mark any services as changed too", func() {
			ctx := context.Background()
			appService := app.Spec.Services[1]
			appUse := app.Spec.Uses[0]
			deploy := appsv1.Deployment{}

			By("First check that it succeeds at getting the deployment")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: ServiceName(app, appService)}, &deploy)
			}, timeout, interval).Should(Succeed())

			configMap := corev1.ConfigMap{}
			secret := corev1.Secret{}

			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: UseConfigName(app, appUse)}, &configMap); err != nil {
				Fail(err.Error())
			}

			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: UseConfigName(app, appUse)}, &secret); err != nil {
				Fail(err.Error())
			}

			app.Spec.Uses[0].Config = map[string]string{"Changed": "Value"}

			sso := ServiceStateObject{ConfigEnvStateObject{nil, app}, nil, appService, app, &deploy, nil, []corev1.ConfigMap{configMap}, []corev1.Secret{secret}}

			Expect(sso.Changed()).To(BeTrue())
		})

		It("Should update a cronjob with a new image", func() {
			ctx := context.Background()

			appCronJob := &app.Spec.CronJobs[0]
			appCronJob.Image = "node"
			// app.Spec.Services[0].Image = "node"

			Expect(k8sClient.Update(ctx, app)).Should(Succeed())

			Eventually(func() string {
				cronJob := batchv1beta.CronJob{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: CronJobName(app, *appCronJob)}, &cronJob)

				if err != nil {
					return err.Error()
				}

				return cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image
			}, timeout, interval).Should(Equal("node"))
		})

		It("Should update a cronjob with a new command", func() {
			ctx := context.Background()

			appCronJob := &app.Spec.CronJobs[0]
			appCronJob.Command = []string{"a", "b", "c"}

			Expect(k8sClient.Update(ctx, app)).Should(Succeed())

			Eventually(func() []string {
				cronJob := batchv1beta.CronJob{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: CronJobName(app, *appCronJob)}, &cronJob)

				if err != nil {
					return []string{err.Error()}
				}

				return cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Command
			}, timeout, interval).Should(BeEquivalentTo(appCronJob.Command))
		})

		It("Should update a cronjob with a new schedule", func() {
			ctx := context.Background()
			app.Spec.CronJobs[0].Schedule = "0 0 * * 0"

			Expect(k8sClient.Update(ctx, app)).Should(Succeed())

			Eventually(func() string {
				cronJob := batchv1beta.CronJob{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: CronJobName(app, app.Spec.CronJobs[0])}, &cronJob)

				if err != nil {
					return err.Error()
				}

				return cronJob.Spec.Schedule
			}, timeout, interval).Should(Equal("0 0 * * 0"))
		})

		// It("Should delete a cronjob if it is removed from the app spec", func() {
		// 	ctx := context.Background()
		// 	cronJob := app.Spec.CronJobs[0]

		// 	Eventually(func() error {
		// 		return k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: CronJobName(app, cronJob)}, &batchv1beta.CronJob{})
		// 	}, timeout, interval).Should(Succeed())

		// 	app.Spec.CronJobs = []hostanv1.AppCronJob{}

		// 	Expect(k8sClient.Update(ctx, app)).Should(Succeed())

		// 	By("And then check that it is deleted")
		// 	Eventually(func() bool {
		// 		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: AppNamespace, Name: CronJobName(app, cronJob)}, &batchv1beta.CronJob{})

		// 		log.Println("got error!!!?!@#!#!@#", err, CronJobName(app, cronJob))
		// 		return errors.IsNotFound(err)
		// 	}, timeout, interval).Should(BeTrue())
		// })
	})

	Context("Delete an app", func() {
		// Apparently the reconciliation loop for orphaned resources does not run in the
		// test environment (which isn't a real k8s cluster), se we'll skip this one for now
	})
})

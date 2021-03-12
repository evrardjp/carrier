package acceptance_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Catalog Services", func() {
	var org = "apps-org"
	var serviceName string
	BeforeEach(func() {
		serviceName = newServiceName()
		setupAndTargetOrg(org)
		setupInClusterServices()
	})

	Describe("create-service", func() {
		It("creates a catalog based service, with waiting", func() {
			makeCatalogService(serviceName)
		})

		It("creates a catalog based service, without waiting", func() {
			makeCatalogServiceDontWait(serviceName)
		})

		AfterEach(func() {
			cleanupService(serviceName)
		})
	})

	Describe("delete service", func() {
		BeforeEach(func() {
			makeCatalogService(serviceName)
		})

		It("deletes a catalog based service", func() {
			deleteService(serviceName)
		})

		It("doesn't delete a bound service", func() {
			appName := newAppName()
			makeApp(appName)
			bindAppService(appName, serviceName, org)

			out, err := Carrier("delete-service "+serviceName, "")
			Expect(err).ToNot(HaveOccurred(), out)

			Expect(out).To(MatchRegexp("Unable to delete service. It is still used by"))
			Expect(out).To(MatchRegexp(appName))
			Expect(out).To(MatchRegexp("Use --unbind to force the issue"))

			verifyAppServiceBound(appName, serviceName, org)

			// Delete again, and force unbind

			out, err = Carrier("delete-service --unbind "+serviceName, "")
			Expect(err).ToNot(HaveOccurred(), out)

			Expect(out).To(MatchRegexp("Unbinding Service From Using Applications Before Deletion"))
			Expect(out).To(MatchRegexp(appName))

			Expect(out).To(MatchRegexp("Unbinding"))
			Expect(out).To(MatchRegexp("Application: " + appName))
			Expect(out).To(MatchRegexp("Unbound"))

			Expect(out).To(MatchRegexp("Service Removed"))

			verifyAppServiceNotbound(appName, serviceName, org)

			// And check non-presence
			Eventually(func() string {
				out, err = Carrier("services", "")
				Expect(err).ToNot(HaveOccurred(), out)
				return out
			}, "10m").ShouldNot(MatchRegexp(serviceName))
		})
	})

	Describe("bind-service", func() {
		var appName string
		BeforeEach(func() {
			appName = newAppName()

			makeCatalogService(serviceName)
			makeApp(appName)
		})

		AfterEach(func() {
			cleanupApp(appName)
			cleanupService(serviceName)
		})

		It("binds a service to the application deployment", func() {
			bindAppService(appName, serviceName, org)
		})
	})

	Describe("unbind-service", func() {
		var appName string
		BeforeEach(func() {
			appName = newAppName()

			makeCatalogService(serviceName)
			makeApp(appName)
			bindAppService(appName, serviceName, org)
		})

		AfterEach(func() {
			cleanupApp(appName)
			cleanupService(serviceName)
		})

		It("unbinds a service from the application deployment", func() {
			unbindAppService(appName, serviceName, org)
		})
	})

	Describe("service", func() {
		BeforeEach(func() {
			makeCatalogService(serviceName)
		})

		It("it shows service details", func() {
			out, err := Carrier("service "+serviceName, "")
			Expect(err).ToNot(HaveOccurred(), out)
			Expect(out).To(MatchRegexp("Service Details"))
			Expect(out).To(MatchRegexp(`Status .*\|.* Provisioned`))
			Expect(out).To(MatchRegexp(`Class .*\|.* mariadb`))
			Expect(out).To(MatchRegexp(`Plan .*\|.* 10-3-22`))
		})

		AfterEach(func() {
			cleanupService(serviceName)
		})
	})
})

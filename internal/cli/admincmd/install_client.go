package admincmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/epinio/epinio/deployments"
	"github.com/epinio/epinio/helpers"
	"github.com/epinio/epinio/helpers/kubernetes"
	"github.com/epinio/epinio/helpers/termui"
	"github.com/epinio/epinio/helpers/tracelog"
	"github.com/epinio/epinio/internal/cli/config"
	"github.com/epinio/epinio/internal/duration"
	"github.com/epinio/epinio/internal/s3manager"
	"github.com/epinio/epinio/internal/version"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InstallClient provides functionality for talking to Kubernetes for
// installing Epinio on it.
type InstallClient struct {
	kubeClient *kubernetes.Cluster
	options    *kubernetes.InstallationOptions
	ui         *termui.UI
	Log        logr.Logger
}

func NewInstallClient(ctx context.Context, options *kubernetes.InstallationOptions) (*InstallClient, func(), error) {
	// We do this for the side effect: colorized output vs not.
	// May also extend the internal CA cert pool.
	// This and everything else done by loading does not matter.
	// The later phases of the install command will write a new config.
	_, err := config.Load()
	if err != nil {
		return nil, nil, err
	}

	cluster, err := kubernetes.GetCluster(ctx)
	if err != nil {
		return nil, nil, err
	}
	uiUI := termui.NewUI()

	logger := tracelog.NewLogger().WithName("EpinioInstaller")
	installClient := &InstallClient{
		kubeClient: cluster,
		ui:         uiUI,
		Log:        logger,
		options:    options,
	}
	return installClient, func() {
	}, nil
}

// Install deploys epinio to the cluster.
func (c *InstallClient) Install(ctx context.Context, flags *pflag.FlagSet) error {
	log := c.Log.WithName("Install")
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	c.ui.Note().Msgf("Epinio %s installing...", version.Version)

	var err error
	details.Info("process cli options")
	c.options, err = c.options.Populate(kubernetes.NewCLIOptionsReader(flags))
	if err != nil {
		return err
	}

	interactive, err := flags.GetBool("interactive")
	if err != nil {
		return err
	}

	if interactive {
		details.Info("query user for options")
		c.options, err = c.options.Populate(kubernetes.NewInteractiveOptionsReader(os.Stdout, os.Stdin))
		if err != nil {
			return err
		}
	} else {
		details.Info("fill defaults into options")
		c.options, err = c.options.Populate(kubernetes.NewDefaultOptionsReader())
		if err != nil {
			return err
		}
	}

	details.Info("show option configuration")
	c.showInstallConfiguration(c.options)

	// TODO (post MVP): Run a validation phase which perform
	// additional checks on the values. For example range limits,
	// proper syntax of the string, etc. do it as pghase, and late
	// to report all problems at once, instead of early and
	// piecemal.

	if err := c.InstallDeployment(ctx, &deployments.Linkerd{
		Timeout: duration.ToDeployment(),
		Log:     details.V(1),
	}, details); err != nil {
		return err
	}

	if err := c.InstallDeployment(ctx, &deployments.Traefik{
		Timeout: duration.ToDeployment(),
		Log:     details.V(1),
	}, details); err != nil {
		return err
	}

	// Try to give a omg.howdoi.website domain if the user didn't specify one
	domain, err := c.options.GetOpt("system_domain", "")
	if err != nil {
		return err
	}

	details.Info("ensure system-domain")
	err = c.fillInMissingSystemDomain(ctx, domain)
	if err != nil {
		return err
	}
	if domain.Value.(string) == "" {
		return errors.New("You didn't provide a system_domain and we were unable to setup a omg.howdoi.website domain (couldn't find an ExternalIP)")
	}

	c.ui.Success().Msg("Using system_domain: " + domain.Value.(string))

	// Validate if ingress svc IP belongs to system domain
	// if it is specified by user
	ingressIP, err := flags.GetString("loadbalancer-ip")
	if err != nil {
		return errors.Wrap(err, "could not read option --loadbalancer-ip")
	}
	if ingressIP != "" {
		bound, err := validateIngressIPDNSBind(domain.Value.(string), ingressIP)
		if err != nil {
			return errors.Wrapf(err, "could not map domain name and ingress service ip address")
		}
		if !bound {
			return errors.New("system domain name is not pointing to ingress service loadbalancer ip address")
		}
	}

	endpoint := c.options.GetStringNG("s3-endpoint")
	key := c.options.GetStringNG("s3-access-key-id")
	secret := c.options.GetStringNG("s3-secret-access-key")
	bucket := c.options.GetStringNG("s3-bucket")
	location := c.options.GetStringNG("s3-location")
	useSSL := c.options.GetBoolNG("s3-use-ssl")
	cd := s3manager.NewConnectionDetails(endpoint, key, secret, bucket, location, useSSL)
	if err := cd.Validate(); err != nil {
		return err
	}
	if endpoint == "" { // All options empty
		cd, err = deployments.MinioInternalConnectionSettings()
		if err != nil {
			return err
		}
	}

	steps := []kubernetes.Deployment{
		&deployments.Kubed{Timeout: duration.ToDeployment()},
		&deployments.CertManager{Timeout: duration.ToDeployment(), Log: details.V(1)},
		&deployments.Epinio{Timeout: duration.ToDeployment()},
		&deployments.Registry{Timeout: duration.ToDeployment(), Log: details.V(1)},
		&deployments.Tekton{Timeout: duration.ToDeployment(), S3ConnectionDetails: cd},
		&deployments.Minio{Timeout: duration.ToDeployment(), Log: details.V(1), S3ConnectionDetails: cd},
	}

	for _, deployment := range steps {
		if err := c.PreInstallCheck(ctx, deployment, details); err != nil {
			return errors.Wrapf(err, "Deployment %s failed pre-installation checks", deployment.ID())
		}
	}

	installationWg := &sync.WaitGroup{}
	for _, deployment := range steps {
		installationWg.Add(1)
		go func(deployment kubernetes.Deployment, wg *sync.WaitGroup) {
			defer wg.Done()
			if err := c.InstallDeployment(ctx, deployment, details); err != nil {
				c.ui.Exclamation().Msgf("Deployment %s failed with error: %v\n", deployment.ID(), err)
				os.Exit(1)
			}
		}(deployment, installationWg)
	}

	installationWg.Wait()

	traefikServiceIngressInfo, err := c.traefikServiceIngressInfo(ctx)
	if err != nil {
		return err
	}

	apiUser, err := c.options.GetString("user", "")
	if err != nil {
		return err
	}

	apiPassword, err := c.options.GetString("password", "")
	if err != nil {
		return err
	}

	c.ui.Success().
		WithStringValue("System domain", domain.Value.(string)).
		WithStringValue("API User", apiUser).
		WithStringValue("API Password", apiPassword).
		WithStringValue("Traefik Ingress info", traefikServiceIngressInfo).
		Msg("Epinio installed.")

	return nil
}

func validateIngressIPDNSBind(systemDomain string, ingressIP string) (bool, error) {
	ips, err := net.LookupIP(systemDomain)
	if err != nil {
		return false, err
	}
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			if ipv4.String() == ingressIP {
				return true, nil
			}
		}
	}
	return false, nil
}

// Uninstall removes epinio from the cluster.
func (c *InstallClient) Uninstall(ctx context.Context) error {
	log := c.Log.WithName("Uninstall")
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	c.ui.Note().Msg("Epinio uninstalling...")

	if err := c.DeleteWorkloads(ctx, c.ui); err != nil {
		return err
	}

	wg := &sync.WaitGroup{}
	for _, deployment := range []kubernetes.Deployment{
		&deployments.Tekton{Timeout: duration.ToDeployment()},
		&deployments.Registry{Timeout: duration.ToDeployment(), Log: details.V(1)},
		&deployments.Kubed{Timeout: duration.ToDeployment()},
		&deployments.Traefik{Timeout: duration.ToDeployment()},
		&deployments.CertManager{Timeout: duration.ToDeployment(), Log: details.V(1)},
		&deployments.Epinio{Timeout: duration.ToDeployment()},
		&deployments.Minio{Timeout: duration.ToDeployment()},
	} {
		wg.Add(1)
		go func(deployment kubernetes.Deployment, wg *sync.WaitGroup) {
			defer wg.Done()
			if err := c.UninstallDeployment(ctx, deployment, details); err != nil {
				c.ui.Exclamation().Msgf("Uninstall of %s failed: %v", deployment.ID(), err)
				os.Exit(1)
			}
			if err := c.PostDeleteCheck(ctx, deployment, details); err != nil {
				c.ui.Exclamation().Msgf("Failed to delete deployment %s", deployment.ID())
			}
		}(deployment, wg)
	}
	wg.Wait()

	if err := c.UninstallDeployment(ctx, &deployments.Linkerd{
		Timeout: duration.ToDeployment(),
		Log:     details.V(1),
	}, details); err != nil {
		c.ui.Exclamation().Msgf("Uninstall of linkerd failed: %v", err)
		os.Exit(1)
	}

	c.ui.Success().Msg("Epinio uninstalled.")

	return nil
}

// InstallIngress deploys epinio's ingress controller to the cluster.
func (c *InstallClient) InstallIngress(cmd *cobra.Command) error {
	log := c.Log.WithName("InstallIngress")
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	ctx := cmd.Context()

	c.ui.Note().Msg("Epinio installing its Ingress (Traefik)...")

	var err error
	details.Info("process cli options")
	c.options, err = c.options.Populate(kubernetes.NewCLIOptionsReader(cmd.Flags()))
	if err != nil {
		return err
	}

	interactive, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return err
	}

	if interactive {
		details.Info("query user for options")
		c.options, err = c.options.Populate(kubernetes.NewInteractiveOptionsReader(os.Stdout, os.Stdin))
		if err != nil {
			return err
		}
	} else {
		details.Info("fill defaults into options")
		c.options, err = c.options.Populate(kubernetes.NewDefaultOptionsReader())
		if err != nil {
			return err
		}
	}

	details.Info("show option configuration")
	c.showInstallConfiguration(c.options)

	// TODO (post MVP): Run a validation phase which perform
	// additional checks on the values. For example range limits,
	// proper syntax of the string, etc. do it as pghase, and late
	// to report all problems at once, instead of early and
	// piecemal.

	if err := c.InstallDeployment(ctx, &deployments.Linkerd{
		Timeout: duration.ToDeployment(),
		Log:     details.V(1),
	}, details); err != nil {
		return err
	}

	if err := c.InstallDeployment(ctx, &deployments.Traefik{
		Timeout: duration.ToDeployment(),
		Log:     details.V(1),
	}, details); err != nil {
		return err
	}

	traefikServiceIngressInfo, err := c.traefikServiceIngressInfo(ctx)
	if err != nil {
		return err
	}

	c.ui.Success().
		WithStringValue("Traefik Ingress info", traefikServiceIngressInfo).
		Msg("Epinio Ingress done.")

	return nil
}

// InstallCertManager deploys epinio's ingress controller to the cluster.
func (c *InstallClient) InstallCertManager(cmd *cobra.Command) error {
	log := c.Log.WithName("InstallCertManager")
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	ctx := cmd.Context()

	c.ui.Note().Msg("Epinio installing cert-manager...")

	var err error
	details.Info("process cli options")
	c.options, err = c.options.Populate(kubernetes.NewCLIOptionsReader(cmd.Flags()))
	if err != nil {
		return err
	}

	interactive, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return err
	}

	if interactive {
		details.Info("query user for options")
		c.options, err = c.options.Populate(kubernetes.NewInteractiveOptionsReader(os.Stdout, os.Stdin))
		if err != nil {
			return err
		}
	} else {
		details.Info("fill defaults into options")
		c.options, err = c.options.Populate(kubernetes.NewDefaultOptionsReader())
		if err != nil {
			return err
		}
	}

	details.Info("show option configuration")
	c.showInstallConfiguration(c.options)

	if err := c.InstallDeployment(ctx, &deployments.CertManager{
		Timeout: duration.ToDeployment(),
		Log:     details.V(1),
	}, details); err != nil {
		return err
	}

	c.ui.Success().Msg("Epinio cert-manager done.")

	return nil
}

func (c *InstallClient) DeleteWorkloads(ctx context.Context, ui *termui.UI) error {
	var nsList *corev1.NamespaceList
	var err error

	nsList, err = c.kubeClient.Kubectl.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", kubernetes.EpinioOrgLabelKey, kubernetes.EpinioOrgLabelValue),
	})
	if err != nil {
		return err
	}

	for _, ns := range nsList.Items {
		ui.Note().KeeplineUnder(1).Msg("Removing namespace " + ns.Name)
		if err := c.kubeClient.Kubectl.CoreV1().Namespaces().
			Delete(ctx, ns.Name, metav1.DeleteOptions{}); err != nil {
			return err
		}
	}
	return nil
}

// PreInstallCheck checks the pre conditions for a single deployment
func (c *InstallClient) PreInstallCheck(ctx context.Context, deployment kubernetes.Deployment, logger logr.Logger) error {
	logger.Info("check", "Deployment", deployment.ID())
	defer logger.Info("return")

	return deployment.PreDeployCheck(ctx, c.kubeClient, c.ui, c.options.ForDeployment(deployment.ID()))
}

// PostDeleteCheck checks the if the deployment was deleted and waits
func (c *InstallClient) PostDeleteCheck(ctx context.Context, deployment kubernetes.Deployment, logger logr.Logger) error {
	logger.Info("check", "Uninstall", deployment.ID())
	defer logger.Info("return")

	return deployment.PostDeleteCheck(ctx, c.kubeClient, c.ui)
}

// InstallDeployment installs one single Deployment on the cluster
func (c *InstallClient) InstallDeployment(ctx context.Context, deployment kubernetes.Deployment, logger logr.Logger) error {
	logger.Info("deploy", "Deployment", deployment.ID())
	defer logger.Info("return")

	return deployment.Deploy(ctx, c.kubeClient, c.ui, c.options.ForDeployment(deployment.ID()))
}

// UninstallDeployment uninstalls one single Deployment from the cluster
func (c *InstallClient) UninstallDeployment(ctx context.Context, deployment kubernetes.Deployment, logger logr.Logger) error {
	logger.Info("remove", "Deployment", deployment.ID())
	return deployment.Delete(ctx, c.kubeClient, c.ui)
}

// showInstallConfiguration prints the options and their values to stdout, to
// inform the user of the detected and chosen configuration
func (c *InstallClient) showInstallConfiguration(opts *kubernetes.InstallationOptions) {
	m := c.ui.Normal()
	for _, opt := range *opts {
		name := "  :compass: " + opt.Name
		switch opt.Type {
		case kubernetes.BooleanType:
			m = m.WithBoolValue(name, opt.Value.(bool))
		case kubernetes.StringType:
			m = m.WithStringValue(name, opt.Value.(string))
		case kubernetes.IntType:
			m = m.WithIntValue(name, opt.Value.(int))
		}
	}
	m.Msg("Configuration...")
}

func (c *InstallClient) fillInMissingSystemDomain(ctx context.Context, domain *kubernetes.InstallationOption) error {
	if domain.Value.(string) == "" {
		ip := ""
		s := c.ui.Progressf("Waiting for LoadBalancer IP on traefik service.")
		defer s.Stop()
		err := helpers.RunToSuccessWithTimeout(
			func() error {
				return c.fetchIP(ctx, &ip)
			}, duration.ToSystemDomain(), duration.PollInterval())
		if err != nil {
			if strings.Contains(err.Error(), "Timed out after") {
				return errors.Wrap(err, deployments.MessageLoadbalancerIP)
			}
			return err
		}

		if ip != "" {
			domain.Value = fmt.Sprintf("%s.omg.howdoi.website", ip)
		}
	}

	return nil
}

func (c *InstallClient) fetchIP(ctx context.Context, ip *string) error {
	serviceList, err := c.kubeClient.Kubectl.CoreV1().Services("").List(ctx, metav1.ListOptions{
		FieldSelector: "metadata.name=traefik",
	})
	if len(serviceList.Items) == 0 {
		return errors.New("couldn't find the traefik service")
	}
	if err != nil {
		return err
	}
	ingress := serviceList.Items[0].Status.LoadBalancer.Ingress
	if len(ingress) <= 0 {
		return errors.New("ingress list is empty in traefik service")
	}
	*ip = ingress[0].IP

	return nil
}

func (c *InstallClient) traefikServiceIngressInfo(ctx context.Context) (string, error) {
	serviceList, err := c.kubeClient.Kubectl.CoreV1().Services("").List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=traefik",
	})
	if err != nil {
		return "", err
	}
	if len(serviceList.Items) == 0 {
		return "not found", nil
	}

	traefikServiceIngressInfo, err := json.Marshal(serviceList.Items[0].Status.LoadBalancer.Ingress)
	if err != nil {
		return "", err
	}

	return string(traefikServiceIngressInfo), nil
}

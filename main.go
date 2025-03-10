// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	kcpclienthelper "github.com/kcp-dev/apimachinery/v2/pkg/client"
	apisv1alpha1 "github.com/kcp-dev/kcp/sdk/apis/apis/v1alpha1"
	"github.com/kcp-dev/logicalcluster/v3"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	clusterapiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/kcp"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/gardener/cluster-api-provider-gardener/controllers/clusters"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme.Scheme))
	utilruntime.Must(clusterapiv1beta1.AddToScheme(scheme.Scheme))
	utilruntime.Must(apisv1alpha1.AddToScheme(scheme.Scheme))
}

func main() {
	var opts Options
	opts.addFlags(flag.CommandLine)
	flag.Parse()
	flag.Lookup("v").Value.Set("6")

	ctx := ctrl.SetupSignalHandler()
	if err := runController(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

type Options struct {
	MetricsAddr          string
	EnableLeaderElection bool
	ProbeAddr            string
	APIExportName        string
	KubeconfigContext    string
}

func (o *Options) addFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.KubeconfigContext, "context", "", "kubeconfig context")
	fs.StringVar(&o.APIExportName, "api-export-name", "cluster-api", "The name of the APIExport.")

	fs.StringVar(&o.MetricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	fs.StringVar(&o.ProbeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	fs.BoolVar(&o.EnableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	klog.InitFlags(fs)
	logf.SetLogger(zap.New())
}

func runController(ctx context.Context, opts Options) error {
	log := ctrl.Log.WithName("setup").WithValues("api-export-name", opts.APIExportName)

	// Important: We use non-controller-runtime client loader so we can always
	// be sure we have correct kubeconfig file. This ease the development and maintenance
	// of the example. In production, you should use the controller-runtime client loader
	// to load the kubeconfig file dedicated to workspace where APIExport is located.
	// restConfig := ctrl.GetConfigOrDie()
	clusterAPICluster := logicalcluster.NewPath("root:cluster-api")
	restConfig, err := config.GetConfigWithContext("base")
	if err != nil {
		return fmt.Errorf("unable to get config: %w", err)
	}
	restConfig = rest.AddUserAgent(kcpclienthelper.SetCluster(restConfig, clusterAPICluster), "kcp-controller-runtime-example")

	ctrlOpts := ctrl.Options{
		HealthProbeBindAddress: opts.ProbeAddr,
		LeaderElection:         opts.EnableLeaderElection,
		LeaderElectionID:       "68a0532d.my.domain",
		LeaderElectionConfig:   restConfig,
	}

	// create a manager, either with or without kcp support
	var mgr ctrl.Manager
	if isKcp, err := kcpAPIsGroupPresent(restConfig); err != nil {
		return fmt.Errorf("error checking for kcp APIs group: %w", err)
	} else if isKcp {
		log.Info("Found KCP APIs, looking up virtual workspace URL")
		exportConfig, err := restConfigForLogicalClusterHostingAPIExport(ctx, restConfig, opts.APIExportName)
		if err != nil {
			return fmt.Errorf("error looking up virtual workspace URL: %w", err)
		}
		log.Info("Using virtual workspace URL", "url", exportConfig.Host)

		mgr, err = kcp.NewClusterAwareManager(exportConfig, ctrlOpts)
		if err != nil {
			return fmt.Errorf("unable to create cluster aware manager: %w", err)
		}
	} else {
		log.Info("The apis.kcp.dev group is not present - creating standard manager")
		mgr, err = ctrl.NewManager(restConfig, ctrlOpts)
		if err != nil {
			return fmt.Errorf("unable to create manager: %w", err)
		}
	}

	// create controllers
	if err = (&clusters.Reconciler{Client: mgr.GetClient()}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create clusters controller: %w", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up ready check: %w", err)
	}

	log.Info("Starting manager")
	return mgr.Start(ctx)
}

// restConfigForLogicalClusterHostingAPIExport returns a *rest.Config properly configured to communicate with the endpoint for the
// APIExport's virtual workspace.
func restConfigForLogicalClusterHostingAPIExport(ctx context.Context, cfg *rest.Config, apiExportName string) (*rest.Config, error) {
	apiExportClient, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("error creating APIExport client: %w", err)
	}

	apiExport := &apisv1alpha1.APIExport{}
	if err := apiExportClient.Get(ctx, types.NamespacedName{Name: apiExportName}, apiExport); err != nil {
		return nil, fmt.Errorf("error getting APIExport %q: %w", apiExportName, err)
	}

	if len(apiExport.Status.VirtualWorkspaces) < 1 {
		return nil, fmt.Errorf("APIExport %q status.virtualWorkspaces is empty", apiExportName)
	}

	// create a new rest.Config with the APIExport's virtual workspace URL
	exportConfig := rest.CopyConfig(cfg)
	exportConfig.Host = apiExport.Status.VirtualWorkspaces[0].URL // TODO(ncdc): sharding support

	return exportConfig, nil
}

func kcpAPIsGroupPresent(cfg *rest.Config) (bool, error) {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return false, fmt.Errorf("failed to create discovery client: %w", err)
	}
	apiGroupList, err := discoveryClient.ServerGroups()
	if err != nil {
		return false, fmt.Errorf("failed to get server groups: %w", err)
	}

	for _, group := range apiGroupList.Groups {
		if group.Name == apisv1alpha1.SchemeGroupVersion.Group {
			for _, version := range group.Versions {
				if version.Version == apisv1alpha1.SchemeGroupVersion.Version {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

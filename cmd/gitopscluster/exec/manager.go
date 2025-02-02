// Copyright 2021 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package exec

import (
	"fmt"
	"os"

	"github.com/IBM/controller-filtered-cache/filteredcache"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"open-cluster-management.io/multicloud-integrations/pkg/apis"
	"open-cluster-management.io/multicloud-integrations/pkg/controller"
	"open-cluster-management.io/multicloud-integrations/pkg/utils"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

// Change below variables to serve metrics on different host or port.
var (
	metricsHost         = "0.0.0.0"
	metricsPort         = 8388
	operatorMetricsPort = 8688
)

// RunManager starts the actual manager
func RunManager() {
	enableLeaderElection := false

	if _, err := rest.InClusterConfig(); err == nil {
		klog.Info("LeaderElection enabled as running in a cluster")

		enableLeaderElection = true
	} else {
		klog.Info("LeaderElection disabled as not running in a cluster")
	}

	// Cache only the managed cluster secrets
	filteredSecretMap := map[schema.GroupVersionKind]filteredcache.Selector{
		v1.SchemeGroupVersion.WithKind("Secret"): {
			LabelSelector: "apps.open-cluster-management.io/cluster-name,argocd.argoproj.io/secret-type==cluster",
		},
	}

	klog.Info("Leader election settings",
		"leaseDuration", options.LeaderElectionLeaseDuration,
		"renewDeadline", options.LeaderElectionRenewDeadline,
		"retryPeriod", options.LeaderElectionRetryPeriod)

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		MetricsBindAddress:      fmt.Sprintf("%s:%d", metricsHost, metricsPort),
		Port:                    operatorMetricsPort,
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        "multicloud-operators-gitopscluster-leader.open-cluster-management.io",
		LeaderElectionNamespace: "kube-system",
		NewCache:                filteredcache.NewFilteredCacheBuilder(filteredSecretMap),
		LeaseDuration:           &options.LeaderElectionLeaseDuration,
		RenewDeadline:           &options.LeaderElectionRenewDeadline,
		RetryPeriod:             &options.LeaderElectionRetryPeriod,
		NewClient:               NewNonCachingClient,
	})

	if err != nil {
		klog.Error(err, "")
		os.Exit(1)
	}

	klog.Info("Registering GitOpsCluster components.")

	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Error(err, "")
		os.Exit(1)
	}

	if err := v1.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Error(err, "")
		os.Exit(1)
	}

	if err := clusterv1beta1.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Error(err, "")
		os.Exit(1)
	}

	if err := clusterv1.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Error(err, "")
		os.Exit(1)
	}

	// Setup all Controllers
	if err := controller.AddGitOpsClusterToManager(mgr); err != nil {
		klog.Error(err, "")
		os.Exit(1)
	}

	sig := signals.SetupSignalHandler()

	klog.Info("Detecting ACM cluster API service...")
	utils.DetectClusterRegistry(sig, mgr.GetAPIReader())

	klog.Info("Starting the Cmd.")

	// Start the Cmd
	if err := mgr.Start(sig); err != nil {
		klog.Error(err, "Manager exited non-zero")
		os.Exit(1)
	}
}

func NewNonCachingClient(cache cache.Cache, config *rest.Config, options client.Options, uncachedObjects ...client.Object) (client.Client, error) {
	return client.New(config, client.Options{Scheme: scheme.Scheme})
}

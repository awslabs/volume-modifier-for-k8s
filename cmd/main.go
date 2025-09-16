package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	csi "github.com/awslabs/volume-modifier-for-k8s/pkg/client"
	"github.com/awslabs/volume-modifier-for-k8s/pkg/controller"
	"github.com/awslabs/volume-modifier-for-k8s/pkg/modifier"
	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	"github.com/kubernetes-csi/external-resizer/pkg/util"
	v1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

var (
	clientConfigUrl = flag.String("client-config-url", "", "URL to build a client config from. Either this or kubeconfig needs to be set if the provisioner is being run out of cluster.")
	kubeConfig      = flag.String("kubeconfig", "", "Absolute path to the kubeconfig")
	resyncPeriod    = flag.Duration("resync-period", time.Minute*10, "Resync period for cache")
	workers         = flag.Int("workers", 10, "Concurrency to process multiple modification requests")

	csiAddress = flag.String("csi-address", "/run/csi/socket", "Address of the CSI driver socket.")
	timeout    = flag.Duration("timeout", 10*time.Second, "Timeout for waiting for CSI driver socket.")

	showVersion = flag.Bool("version", false, "Show version")

	retryIntervalStart = flag.Duration("retry-interval-start", time.Second, "Initial retry interval of failed volume modification. It exponentially increases with each failure, up to retry-interval-max.")
	retryIntervalMax   = flag.Duration("retry-interval-max", 5*time.Minute, "Maximum retry interval of failed volume modification.")

	enableLeaderElection        = flag.Bool("leader-election", false, "Enable leader election.")
	leaderElectionNamespace     = flag.String("leader-election-namespace", "", "Namespace where the leader election resource lives. Defaults to the pod namespace if not set.")
	leaderElectionLeaseDuration = flag.Duration("leader-election-lease-duration", 15*time.Second, "Duration, in seconds, that non-leader candidates will wait to force acquire leadership. Defaults to 15 seconds.")
	leaderElectionRenewDeadline = flag.Duration("leader-election-renew-deadline", 10*time.Second, "Duration, in seconds, that the acting leader will retry refreshing leadership before giving up. Defaults to 10 seconds.")
	leaderElectionRetryPeriod   = flag.Duration("leader-election-retry-period", 5*time.Second, "Duration, in seconds, the LeaderElector clients should wait between tries of actions. Defaults to 5 seconds.")

	httpEndpoint = flag.String("http-endpoint", "", "The TCP network address where the HTTP server for diagnostics, including metrics and leader election health check, will listen (example: `:8080`). The default is empty string, which means the server is disabled. Only one of `--metrics-address` and `--http-endpoint` can be set.")
	metricsPath  = flag.String("metrics-path", "/metrics", "The HTTP path where prometheus metrics will be exposed. Default is `/metrics`.")

	kubeAPIQPS   = flag.Float64("kube-api-qps", 5, "QPS to use while communicating with the kubernetes apiserver. Defaults to 5.0.")
	kubeAPIBurst = flag.Int("kube-api-burst", 10, "Burst to use while communicating with the kubernetes apiserver. Defaults to 10.")

	// Passed through ldflags.
	version = "<unknown>"
)

func main() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	flag.Parse()

	if *showVersion {
		fmt.Println(os.Args[0], version)
		os.Exit(0)
	}
	klog.Infof("Version : %s", version)
	klog.InfoS("Leader election must be enabled in the external-resizer CSI sidecar")

	// https://github.com/kubernetes-csi/csi-lib-utils/blob/master/leaderelection/leader_election.go#L212-L214
	leaseIdentity, err := os.Hostname()
	if err != nil {
		klog.Fatal("Failed to get hostname for lease identity", "err", err)
	}
	podNamespace := os.Getenv("POD_NAMESPACE")
	if podNamespace == "" {
		klog.Fatal("POD_NAMESPACE environment variable is not set")
	}

	addr := *httpEndpoint
	var config *rest.Config
	if *clientConfigUrl != "" || *kubeConfig != "" {
		config, err = clientcmd.BuildConfigFromFlags(*clientConfigUrl, *kubeConfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		klog.Fatal(err.Error())
	}

	config.QPS = float32(*kubeAPIQPS)
	config.Burst = *kubeAPIBurst

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatal(err.Error())
	}

	mux := http.NewServeMux()
	metricsManager := metrics.NewCSIMetricsManager("" /* driverName */)
	csiClient, err := csi.New(*csiAddress, *timeout, metricsManager)
	if err != nil {
		klog.Fatal(err.Error())
	}
	if err := csiClient.SupportsVolumeModification(context.TODO()); err != nil {
		klog.Fatalf("CSI driver does not support volume modification: %v", err)
	}

	driverName, err := getDriverName(csiClient, *timeout)
	if err != nil {
		klog.Fatal(fmt.Errorf("get driver name failed: %v", err))
	}
	klog.V(2).Infof("CSI driver name: %q", driverName)

	csiModifier, err := modifier.NewFromClient(
		driverName,
		csiClient,
		kubeClient,
		*timeout,
	)
	if err != nil {
		klog.Fatal(err.Error())
	}

	if addr != "" {
		metricsManager.RegisterToServer(mux, *metricsPath)
		metricsManager.SetDriverName(driverName)
		go func() {
			klog.Infof("ServeMux listening at %q", addr)
			err := http.ListenAndServe(addr, mux)
			if err != nil {
				klog.Fatalf("Failed to start HTTP server at specified address (%q) and metrics path (%q): %s", addr, *metricsPath, err)
			}
		}()
	}

	modifierName := csiModifier.Name()
	mc := func() controller.ModifyController {
		return controller.NewModifyController(
			modifierName,
			csiModifier,
			kubeClient,
			*resyncPeriod,
			informers.NewSharedInformerFactory(kubeClient, *resyncPeriod),
			workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax),
			true, /* retryFailure */
		)
	}
	leaseChannel := make(chan *v1.Lease)
	go leaseHandler(leaseIdentity, mc, leaseChannel)

	informerFactoryLeases := informers.NewSharedInformerFactoryWithOptions(kubeClient, *resyncPeriod, informers.WithNamespace(podNamespace))
	leaseInformer := informerFactoryLeases.Coordination().V1().Leases().Informer()
	leaseInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj interface{}) {
			lease, ok := newObj.(*v1.Lease)
			if !ok {
				klog.ErrorS(nil, "Failed to process object, expected it to be a Lease", "obj", newObj)
				return
			}
			resizerLeaseName := "external-resizer-" + util.SanitizeName(driverName)
			if lease.Name == resizerLeaseName {
				leaseChannel <- lease
			}
		},
	})
	informerFactoryLeases.Start(wait.NeverStop)
	leaseInformer.Run(wait.NeverStop)
}

func leaseHandler(leaseIdentity string, mc func() controller.ModifyController, leaseChannel chan *v1.Lease) {
	var cancel context.CancelFunc = nil

	klog.InfoS("leaseHandler: Looking for external-resizer lease holder")

	timer := time.NewTimer(*resyncPeriod)
	defer timer.Stop()

	for {
		select {
		case lease, ok := <-leaseChannel:
			if !ok {
				if cancel != nil {
					cancel()
				}
				return
			}
			currentLeader := *lease.Spec.HolderIdentity
			klog.V(6).InfoS("leaseHandler: Lease updated", "currentLeader", currentLeader, "leaseIdentity", leaseIdentity)

			if currentLeader == leaseIdentity && cancel == nil {
				var ctx context.Context
				ctx, cancel = context.WithCancel(context.Background())
				klog.InfoS("leaseHandler: Starting ModifyController", "leaseIdentity", leaseIdentity, "currentLeader", currentLeader)
				go mc().Run(*workers, ctx)
			} else if currentLeader != leaseIdentity && cancel != nil {
				klog.InfoS("leaseHandler: Stopping ModifyController", "leaseIdentity", leaseIdentity, "currentLeader", currentLeader)
				cancel()
				cancel = nil
			}

			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(*resyncPeriod)

		case <-timer.C:
			if cancel != nil {
				cancel()
			}
			klog.Fatalf("leaseHandler: No external-resizer lease update received within timeout period. Timeout: %v", *resyncPeriod)
		}
	}
}

func getDriverName(client csi.Client, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return client.GetDriverName(ctx)
}

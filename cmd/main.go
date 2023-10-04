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

	podName := os.Getenv("POD_NAME")
	if podName == "" {
		klog.Fatal("POD_NAME environment variable is not set")
	}

	addr := *httpEndpoint
	var config *rest.Config
	var err error
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

	informerFactory := informers.NewSharedInformerFactory(kubeClient, *resyncPeriod)
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
	mc := controller.NewModifyController(
		modifierName,
		csiModifier,
		kubeClient,
		*resyncPeriod,
		informerFactory,
		workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax),
		true, /* retryFailure */
	)
	leaseChannel := make(chan *v1.Lease)
	go leaseHandler(podName, mc, leaseChannel)

	leaseInformer := informerFactory.Coordination().V1().Leases().Informer()
	leaseInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj interface{}) {
			lease, ok := newObj.(*v1.Lease)
			if !ok {
				klog.ErrorS(nil, "Failed to process object, expected it to be a Lease", "obj", newObj)
				return
			}
			if lease.Name == "external-resizer-ebs-csi-aws-com" {
				leaseChannel <- lease
			}
		},
	})
	informerFactory.Start(wait.NeverStop)
	leaseInformer.Run(wait.NeverStop)
}

func leaseHandler(podName string, mc controller.ModifyController, leaseChannel chan *v1.Lease) {
	var cancel context.CancelFunc = nil

	for lease := range leaseChannel {
		currentLeader := *lease.Spec.HolderIdentity

		klog.V(6).InfoS("leaseHandler: Lease updated", "currentLeader", currentLeader, "podName", podName)

		if currentLeader == podName && cancel == nil {
			var ctx context.Context
			ctx, cancel = context.WithCancel(context.Background())
			klog.InfoS("leaseHandler: Starting ModifyController", "podName", podName, "currentLeader", currentLeader)
			go mc.Run(*workers, ctx)
		} else if currentLeader != podName && cancel != nil {
			klog.InfoS("leaseHandler: Stopping ModifyController", "podName", podName, "currentLeader", currentLeader)
			cancel()
			cancel = nil
		}
	}

	// Ensure cancel is called if it's not nil when we exit the function
	if cancel != nil {
		cancel()
	}
}

func getDriverName(client csi.Client, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return client.GetDriverName(ctx)
}

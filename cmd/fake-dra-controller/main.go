package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"path"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	coreclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/component-base/cli"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/featuregate"
	"k8s.io/component-base/logs"
	logsapi "k8s.io/component-base/logs/api/v1"
	"k8s.io/component-base/term"
	"k8s.io/dynamic-resource-allocation/controller"
	"k8s.io/klog/v2"

	_ "k8s.io/component-base/logs/json/register" // for JSON log output support
	"k8s.io/component-base/metrics/legacyregistry"
	_ "k8s.io/component-base/metrics/prometheus/restclient" // for client metric registration
	_ "k8s.io/component-base/metrics/prometheus/version"    // for version metric registration
	_ "k8s.io/component-base/metrics/prometheus/workqueue"  // register work queues in the default legacy registry

	shakeclientset "github.com/toVersus/fake-dra-driver/pkg/3-shake.com/resource/clientset/versioned"
)

type Flags struct {
	kubeconfig   *string
	kubeAPIQPS   *float32
	kubeAPIBurst *int
	workers      *int

	httpEndpoint *string
	metricsPath  *string
	profilePath  *string
}

type Clientset struct {
	core  coreclientset.Interface
	shake shakeclientset.Interface
}

type Config struct {
	namespace string
	flags     *Flags
	csconfig  *rest.Config
	clientset *Clientset
	ctx       context.Context
	mux       *http.ServeMux
}

func main() {
	command := NewCommand()
	code := cli.Run(command)
	os.Exit(code)
}

func NewCommand() *cobra.Command {
	featureGate := featuregate.NewFeatureGate()
	logsconfig := logsapi.NewLoggingConfiguration()
	utilruntime.Must(logsapi.AddFeatureGates(featureGate))

	cmd := &cobra.Command{
		Use:  "fake-dra-controller",
		Long: "fake-dra-controller is a Kubernetes controller that implements the Dynamic Resource Allocation API based driver",
	}
	flags := AddFlags(cmd, logsconfig, featureGate)

	logger := klog.Background().WithName("fake-dra-controller")
	ctx := klog.NewContext(context.Background(), logger)

	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		v := viper.New()
		v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
		v.AutomaticEnv()
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if !f.Changed && v.IsSet(f.Name) {
				val := v.Get(f.Name)
				if err := cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val)); err != nil {
					logger.Error(err, "Unable to bind environment variable to input flag", "key", f.Name, "value", val)
				}
			}
		})
		if err := logsapi.ValidateAndApply(logsconfig, featureGate); err != nil {
			return err
		}
		return nil
	}

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		mux := http.NewServeMux()

		csconfig, err := GetClusterConfig(ctx, flags)
		if err != nil {
			return fmt.Errorf("error creating client configuration: %w", err)
		}

		coreclient, err := coreclientset.NewForConfig(csconfig)
		if err != nil {
			return fmt.Errorf("error creating core client: %w", err)
		}

		shakeclient, err := shakeclientset.NewForConfig(csconfig)
		if err != nil {
			return fmt.Errorf("error creating shake client: %w", err)
		}

		config := &Config{
			ctx:       ctx,
			mux:       mux,
			flags:     flags,
			csconfig:  csconfig,
			namespace: os.Getenv("POD_NAMESPACE"),
			clientset: &Clientset{
				coreclient,
				shakeclient,
			},
		}

		if *flags.httpEndpoint != "" {
			if err = SetupHTTPEndpoint(ctx, config); err != nil {
				return fmt.Errorf("error creating HTTP endpoint: %w", err)
			}
		}

		err = StartClaimParametersGenerator(ctx, config)
		if err != nil {
			return fmt.Errorf("start claim parameters generator: %w", err)
		}

		err = StartController(ctx, config)
		if err != nil {
			return fmt.Errorf("start controller: %w", err)
		}

		return nil
	}

	return cmd
}

func AddFlags(cmd *cobra.Command, logsconfig *logsapi.LoggingConfiguration, featureGate featuregate.MutableFeatureGate) *Flags {
	flags := &Flags{}
	sharedFlagSets := cliflag.NamedFlagSets{}

	fs := sharedFlagSets.FlagSet("logging")
	logsapi.AddFlags(logsconfig, fs)
	logs.AddFlags(fs, logs.SkipLoggingConfigurationFlags())

	fs = sharedFlagSets.FlagSet("Kubernetes client")
	flags.kubeconfig = fs.String("kubeconfig", "", "Absolute path to the kube.config file. Either this or KUBECONFIG need to be set if the driver is being run out of cluster.")
	flags.kubeAPIQPS = fs.Float32("kube-api-qps", 5, "QPS to use while communicating with the kubernetes apiserver.")
	flags.kubeAPIBurst = fs.Int("kube-api-burst", 10, "Burst to use while communicating with the kubernetes apiserver.")
	flags.workers = fs.Int("workers", 10, "Concurrency to process multiple claims")

	fs = sharedFlagSets.FlagSet("http server")
	flags.httpEndpoint = fs.String("http-endpoint", "",
		"The TCP network address where the HTTP server for diagnostics, including pprof and metrics will listen (example: `:8080`). The default is the empty string, which means the server is disabled.")
	flags.metricsPath = fs.String("metrics-path", "/metrics", "The HTTP path where Prometheus metrics will be exposed, disabled if empty.")
	flags.profilePath = fs.String("pprof-path", "", "The HTTP path where pprof profiling will be available, disabled if empty.")

	fs = sharedFlagSets.FlagSet("other")
	featureGate.AddFlag(fs)

	fs = cmd.PersistentFlags()
	for _, f := range sharedFlagSets.FlagSets {
		fs.AddFlagSet(f)
	}

	cols, _, _ := term.TerminalSize(cmd.OutOrStdout())
	cliflag.SetUsageAndHelpFunc(cmd, sharedFlagSets, cols)

	return flags
}

func GetClusterConfig(ctx context.Context, f *Flags) (*rest.Config, error) {
	logger := klog.FromContext(ctx)
	var csconfig *rest.Config

	kubeconfigEnv := os.Getenv("KUBECONFIG")
	if kubeconfigEnv != "" {
		logger.Info("Found KUBECONFIG environment variable set, using that...")
		*f.kubeconfig = kubeconfigEnv
	}

	var err error
	if *f.kubeconfig == "" {
		csconfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("create in-cluster client configuration: %w", err)
		}
	} else {
		csconfig, err = clientcmd.BuildConfigFromFlags("", *f.kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("create out-of-cluster client configuration from kubeconfig: %w", err)
		}
	}

	csconfig.QPS = *f.kubeAPIQPS
	csconfig.Burst = *f.kubeAPIBurst

	return csconfig, nil
}

func SetupHTTPEndpoint(ctx context.Context, config *Config) error {
	logger := klog.FromContext(ctx)
	if *config.flags.metricsPath != "" {
		// To collect metrics data from the metric handler itself, we
		// let it register itself and then collect from that registry.
		reg := prometheus.NewRegistry()
		gatherers := prometheus.Gatherers{
			// Include Go runtime and process metrics:
			// https://github.com/kubernetes/kubernetes/blob/9780d88cb6a4b5b067256ecb4abf56892093ee87/staging/src/k8s.io/component-base/metrics/legacyregistry/registry.go#L46-L49
			legacyregistry.DefaultGatherer,
		}
		gatherers = append(gatherers, reg)

		actualPath := path.Join("/", *config.flags.metricsPath)
		logger.Info("Starting metrics", "path", actualPath)
		// This is similar to k8s.io/component-base/metrics HandlerWithReset
		// except that we gather from multiple sources.
		config.mux.Handle(actualPath,
			promhttp.InstrumentMetricHandler(
				reg,
				promhttp.HandlerFor(gatherers, promhttp.HandlerOpts{})))
	}

	if *config.flags.profilePath != "" {
		actualPath := path.Join("/", *config.flags.profilePath)
		logger.Info("Starting profiling", "path", actualPath)
		config.mux.HandleFunc(path.Join("/", *config.flags.profilePath), pprof.Index)
		config.mux.HandleFunc(path.Join("/", *config.flags.profilePath, "cmdline"), pprof.Cmdline)
		config.mux.HandleFunc(path.Join("/", *config.flags.profilePath, "profile"), pprof.Profile)
		config.mux.HandleFunc(path.Join("/", *config.flags.profilePath, "symbol"), pprof.Symbol)
		config.mux.HandleFunc(path.Join("/", *config.flags.profilePath, "trace"), pprof.Trace)
	}

	listener, err := net.Listen("tcp", *config.flags.httpEndpoint)
	if err != nil {
		return fmt.Errorf("Listen on HTTP endpoint: %v", err)
	}

	go func() {
		logger.Info("Starting HTTP server", "endpoint", *config.flags.httpEndpoint)
		err := http.Serve(listener, config.mux)
		if err != nil {
			logger.Error(err, "HTTP server failed")
			klog.FlushAndExit(klog.ExitFlushTimeout, 1)
		}
	}()

	return nil
}

func StartController(ctx context.Context, config *Config) error {
	driver := NewDriver(config)
	informerFactory := informers.NewSharedInformerFactory(config.clientset.core, 0 /* resync period */)
	ctrl := controller.New(config.ctx, DriverAPIGroup, driver, config.clientset.core, informerFactory)
	informerFactory.Start(config.ctx.Done())
	ctrl.Run(*config.flags.workers)
	return nil
}

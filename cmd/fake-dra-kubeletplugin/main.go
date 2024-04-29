package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/featuregate"
	"k8s.io/component-base/logs"
	logsapi "k8s.io/component-base/logs/api/v1"
	"k8s.io/component-base/term"
	plugin "k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/klog/v2"

	_ "k8s.io/component-base/logs/json/register" // for JSON log output support

	fakecrd "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/v1alpha1"
	shakeclientset "github.com/toVersus/fake-dra-driver/pkg/3-shake.com/resource/clientset/versioned"
)

const (
	DriverName     = fakecrd.GroupName
	DriverAPIGroup = fakecrd.GroupName

	PluginRegistrationPath = "/var/lib/kubelet/plugins_registry/" + DriverName + ".sock"
	DriverPluginPath       = "/var/lib/kubelet/plugins/" + DriverName
	DriverPluginSocketPath = DriverPluginPath + "/plugin.sock"
)

type Flags struct {
	kubeconfig   *string
	kubeAPIQPS   *float32
	kubeAPIBurst *int

	cdiRoot *string
}

type Config struct {
	flags       *Flags
	shakeclient shakeclientset.Interface
}

func main() {
	command := NewCommand()
	if err := command.Execute(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}

}

func NewCommand() *cobra.Command {
	featureGate := featuregate.NewFeatureGate()
	logsconfig := logsapi.NewLoggingConfiguration()
	utilruntime.Must(logsapi.AddFeatureGates(featureGate))

	cmd := &cobra.Command{
		Use:  "fake-dra-kubeletplugin",
		Long: "fake-dra-kubeletplugin implements the Dynamic Resource Allocation API based kubelet plugin",
	}

	flags := AddFlags(cmd, logsconfig)

	logger := klog.Background().WithName("fake-dra-kubeletplugin")
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
		csconfig, err := GetClientsetConfig(ctx, flags)
		if err != nil {
			return fmt.Errorf("error creating client configuration: %w", err)
		}

		shakeclient, err := shakeclientset.NewForConfig(csconfig)
		if err != nil {
			return fmt.Errorf("error creating 3-shake.com client: %w", err)
		}

		nodeName := os.Getenv("NODE_NAME")
		podNamespace := os.Getenv("POD_NAMESPACE")

		config := &Config{
			flags:       flags,
			shakeclient: shakeclient,
		}

		klog.InfoS("Starting fake-dra-kubeletplugin", "pod", podNamespace, "node", nodeName)
		return StartPlugin(ctx, config)
	}

	return cmd
}

func AddFlags(cmd *cobra.Command, logsconfig *logsapi.LoggingConfiguration) *Flags {
	flags := &Flags{}
	sharedFlagSets := cliflag.NamedFlagSets{}

	fs := sharedFlagSets.FlagSet("logging")
	logsapi.AddFlags(logsconfig, fs)
	logs.AddFlags(fs, logs.SkipLoggingConfigurationFlags())

	fs = sharedFlagSets.FlagSet("Kubernetes client")
	flags.kubeconfig = fs.String("kubeconfig", "", "Absolute path to the kube.config file. Either this or KUBECONFIG need to be set if the driver is being run out of cluster.")
	flags.kubeAPIQPS = fs.Float32("kube-api-qps", 5, "QPS to use while communicating with the kubernetes apiserver.")
	flags.kubeAPIBurst = fs.Int("kube-api-burst", 10, "Burst to use while communicating with the kubernetes apiserver.")

	fs = sharedFlagSets.FlagSet("CDI")
	flags.cdiRoot = fs.String("cdi-root", "/etc/cdi", "Absolute path to the directory where CDI files will be generated.")

	fs = cmd.PersistentFlags()
	for _, f := range sharedFlagSets.FlagSets {
		fs.AddFlagSet(f)
	}

	cols, _, _ := term.TerminalSize(cmd.OutOrStdout())
	cliflag.SetUsageAndHelpFunc(cmd, sharedFlagSets, cols)

	return flags
}

func GetClientsetConfig(ctx context.Context, f *Flags) (*rest.Config, error) {
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
			return nil, fmt.Errorf("create in-cluster client configuration: %v", err)
		}
	} else {
		csconfig, err = clientcmd.BuildConfigFromFlags("", *f.kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("create out-of-cluster client configuration: %v", err)
		}
	}

	csconfig.QPS = *f.kubeAPIQPS
	csconfig.Burst = *f.kubeAPIBurst

	return csconfig, nil
}

func StartPlugin(ctx context.Context, config *Config) error {
	logger := klog.FromContext(ctx)
	logger.Info("Starting fake-dra-kubeletplugin")

	logger.Info("Creating plugin directory", "dir", DriverPluginPath)
	if err := os.MkdirAll(DriverPluginPath, 0750); err != nil {
		return fmt.Errorf("error creating plugin directory: %w", err)
	}

	info, err := os.Stat(*config.flags.cdiRoot)
	if err != nil && os.IsNotExist(err) {
		logger.Info("Creating CDI config directory", "dir", *config.flags.cdiRoot)
		if err := os.MkdirAll(*config.flags.cdiRoot, 0750); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !info.IsDir() {
		return fmt.Errorf("path to cdi file file generation must not be a directory: %w", err)
	}

	driver, err := NewDriver(ctx, config)
	if err != nil {
		return err
	}

	dp, err := plugin.Start(
		driver,
		plugin.DriverName(DriverName),
		plugin.RegistrarSocketPath(PluginRegistrationPath),
		plugin.PluginSocketPath(DriverPluginSocketPath),
		plugin.KubeletPluginSocketPath(DriverPluginSocketPath),
	)
	if err != nil {
		return err
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	klog.Info("Got signal, shutting down fake-dra-kubeletplugin...", "signal", <-sig)
	<-sig

	dp.Stop()

	if err := driver.Shutdown(ctx); err != nil {
		return fmt.Errorf("error shutting down driver: %w", err)
	}
	logger.Info("Shutdown fake-dra-kubeletplugin completed successfully")

	return nil
}

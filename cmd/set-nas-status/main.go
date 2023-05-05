/*
 * Copyright 2023 The Kubernetes Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/featuregate"
	"k8s.io/component-base/logs"
	logsapi "k8s.io/component-base/logs/api/v1"
	"k8s.io/component-base/term"
	"k8s.io/klog/v2"

	nascrd "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/nas/v1alpha1"
	nasclient "github.com/toVersus/fake-dra-driver/api/3-shake.com/resource/fake/nas/v1alpha1/client"
	shakeclientset "github.com/toVersus/fake-dra-driver/pkg/3-shake.com/resource/clientset/versioned"

	_ "k8s.io/component-base/logs/json/register" // for JSON log output support
)

type Flags struct {
	kubeconfig *string
	status     *string
}

type Config struct {
	flags       *Flags
	nascrd      *nascrd.NodeAllocationState
	shakeclient shakeclientset.Interface
}

func main() {
	command := NewCommand()
	err := command.Execute()
	if err != nil {
		log.Fatalf("Failed to update status of NodeAllocationState CRD: %s", err)
	}
}

// NewCommand creates a *cobra.Command object with default parameters.
func NewCommand() *cobra.Command {
	featureGate := featuregate.NewFeatureGate()
	logsconfig := logsapi.NewLoggingConfiguration()
	utilruntime.Must(logsapi.AddFeatureGates(featureGate))

	cmd := &cobra.Command{
		Use:  "set-nas-status",
		Long: "set-nas-status sets the status of the NodeAllocationState CRD managed by the DRA driver for GPUs.",
	}

	flags := AddFlags(cmd, logsconfig)

	logger := klog.Background().WithName("fake-dra-kubeletplugin")

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
		err := ValidateFlags(flags)
		if err != nil {
			return fmt.Errorf("validate flags: %v", err)
		}

		csconfig, err := GetClientsetConfig(flags)
		if err != nil {
			return fmt.Errorf("create client configuration: %v", err)
		}

		coreclient, err := coreclientset.NewForConfig(csconfig)
		if err != nil {
			return fmt.Errorf("create core client: %v", err)
		}

		shakeclient, err := shakeclientset.NewForConfig(csconfig)
		if err != nil {
			return fmt.Errorf("create example.com client: %v", err)
		}

		nodeName := os.Getenv("NODE_NAME")
		podNamespace := os.Getenv("POD_NAMESPACE")

		node, err := coreclient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get node object: %v", err)
		}

		crdconfig := &nascrd.NodeAllocationStateConfig{
			Name:      nodeName,
			Namespace: podNamespace,
			Owner: &metav1.OwnerReference{
				APIVersion: "v1",
				Kind:       "Node",
				Name:       nodeName,
				UID:        node.UID,
			},
		}
		nascrd := nascrd.NewNodeAllocationState(crdconfig)

		config := &Config{
			flags:       flags,
			nascrd:      nascrd,
			shakeclient: shakeclient,
		}

		return SetStatus(config)
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
	flags.status = fs.String("status", "", "The status to set [Ready | NotReady].")

	fs = cmd.PersistentFlags()
	for _, f := range sharedFlagSets.FlagSets {
		fs.AddFlagSet(f)
	}

	cols, _, _ := term.TerminalSize(cmd.OutOrStdout())
	cliflag.SetUsageAndHelpFunc(cmd, sharedFlagSets, cols)

	return flags
}

func ValidateFlags(f *Flags) error {
	switch strings.ToLower(*f.status) {
	case strings.ToLower(nascrd.NodeAllocationStateStatusReady):
		*f.status = nascrd.NodeAllocationStateStatusReady
	case strings.ToLower(nascrd.NodeAllocationStateStatusNotReady):
		*f.status = nascrd.NodeAllocationStateStatusNotReady
	default:
		return fmt.Errorf("unknown status: %v", *f.status)
	}
	return nil
}

func GetClientsetConfig(f *Flags) (*rest.Config, error) {
	var csconfig *rest.Config

	kubeconfigEnv := os.Getenv("KUBECONFIG")
	if kubeconfigEnv != "" {
		klog.Infof("Found KUBECONFIG environment variable set, using that..")
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

	return csconfig, nil
}

func SetStatus(config *Config) error {
	client := nasclient.New(config.nascrd, config.shakeclient.NasV1alpha1())

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := client.GetOrCreate()
		if err != nil {
			return err
		}

		return client.UpdateStatus(*config.flags.status)
	})
	if err != nil {
		return err
	}

	return nil
}

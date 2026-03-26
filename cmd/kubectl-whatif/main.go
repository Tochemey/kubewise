// Copyright 2026 KubeWise Authors
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

package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

// Build-time variables populated via ldflags.
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

// Global flags shared by all subcommands.
var (
	kubeconfig    string
	kubeContext   string
	namespace     string
	prometheusURL string
	outputFormat  string
	verbose       bool
	noColor       bool
)

func init() {
	klog.InitFlags(nil)
}

func main() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kubectl-whatif",
		Short: "Kubernetes cost and performance what-if simulator",
		Long: `KubeWise is a Kubernetes cost × performance "what-if" simulator.
It snapshots a live cluster's state, applies hypothetical changes,
simulates the outcome, and reports cost savings alongside reliability risk.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPostRun: func(_ *cobra.Command, _ []string) {
			klog.Flush()
		},
	}

	// Global persistent flags
	pf := cmd.PersistentFlags()
	pf.StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig (default: $KUBECONFIG or ~/.kube/config)")
	pf.StringVar(&kubeContext, "context", "", "Kubernetes context to use")
	pf.StringVar(&namespace, "namespace", "", "Limit to a specific namespace (default: all)")
	pf.StringVar(&prometheusURL, "prometheus-url", "", "Prometheus endpoint (auto-discovered if not set)")
	pf.StringVarP(&outputFormat, "output", "o", "table", "Output format: table, json, markdown")
	pf.BoolVar(&verbose, "verbose", false, "Show detailed per-workload breakdown")
	pf.BoolVar(&noColor, "no-color", false, "Disable terminal colors")

	// Register subcommands
	cmd.AddCommand(newSnapshotCmd())
	cmd.AddCommand(newRightSizeCmd())
	cmd.AddCommand(newConsolidateCmd())
	cmd.AddCommand(newSpotCmd())
	cmd.AddCommand(newApplyCmd())
	cmd.AddCommand(newCompareCmd())
	cmd.AddCommand(newVersionCmd())

	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Printf("kubectl-whatif %s\n", version)
			fmt.Printf("  commit:     %s\n", commit)
			fmt.Printf("  build date: %s\n", buildDate)
			fmt.Printf("  go version: %s\n", runtime.Version())
			fmt.Printf("  platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
		},
	}
}

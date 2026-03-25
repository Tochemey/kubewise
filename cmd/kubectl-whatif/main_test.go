//    Copyright 2026 KubeWise Authors
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package main

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootCommandStructure(t *testing.T) {
	root := newRootCmd()

	assert.Equal(t, "kubectl-whatif", root.Use)
	assert.NotEmpty(t, root.Short)
	assert.NotEmpty(t, root.Long)
}

func TestSubcommandsRegistered(t *testing.T) {
	root := newRootCmd()

	commands := make(map[string]bool)
	for _, cmd := range root.Commands() {
		commands[cmd.Name()] = true
	}

	assert.True(t, commands["snapshot"], "snapshot command should be registered")
	assert.True(t, commands["rightsize"], "rightsize command should be registered")
	assert.True(t, commands["consolidate"], "consolidate command should be registered")
	assert.True(t, commands["spot"], "spot command should be registered")
	assert.True(t, commands["apply"], "apply command should be registered")
	assert.True(t, commands["compare"], "compare command should be registered")
	assert.True(t, commands["version"], "version command should be registered")
}

func TestGlobalFlagsRegistered(t *testing.T) {
	root := newRootCmd()

	flags := []string{"kubeconfig", "context", "namespace", "prometheus-url", "output", "verbose", "no-color"}
	for _, flag := range flags {
		f := root.PersistentFlags().Lookup(flag)
		assert.NotNil(t, f, "global flag --%s should be registered", flag)
	}
}

func TestOutputFlagShorthand(t *testing.T) {
	root := newRootCmd()
	f := root.PersistentFlags().ShorthandLookup("o")
	assert.NotNil(t, f)
	assert.Equal(t, "output", f.Name)
}

func TestOutputFlagDefault(t *testing.T) {
	root := newRootCmd()
	f := root.PersistentFlags().Lookup("output")
	require.NotNil(t, f)
	assert.Equal(t, "table", f.DefValue)
}

func TestRightSizeFlagsRegistered(t *testing.T) {
	root := newRootCmd()

	// Find the rightsize command
	var rsCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "rightsize" {
			rsCmd = cmd
			break
		}
	}
	require.NotNil(t, rsCmd)

	flags := map[string]string{
		"percentile":        "p95",
		"buffer":            "20",
		"scope":             "",
		"exclude-namespace": "kube-system",
		"limit-strategy":    "ratio",
	}
	for flag, defaultVal := range flags {
		f := rsCmd.Flags().Lookup(flag)
		assert.NotNil(t, f, "rightsize flag --%s should be registered", flag)
		if defaultVal != "" {
			assert.Equal(t, defaultVal, f.DefValue, "rightsize flag --%s default", flag)
		}
	}
}

func TestSnapshotFlagsRegistered(t *testing.T) {
	root := newRootCmd()

	var snapCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "snapshot" {
			snapCmd = cmd
			break
		}
	}
	require.NotNil(t, snapCmd)

	f := snapCmd.Flags().Lookup("save")
	assert.NotNil(t, f, "snapshot --save flag should be registered")
}

func TestVersionCommandRuns(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"version"})
	err := root.Execute()
	assert.NoError(t, err)
}

func TestConsolidateFlagsRegistered(t *testing.T) {
	root := newRootCmd()
	var consCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "consolidate" {
			consCmd = cmd
			break
		}
	}
	require.NotNil(t, consCmd)

	flags := []string{"node-type", "max-nodes", "keep-pool", "target-cpu", "target-memory"}
	for _, flag := range flags {
		f := consCmd.Flags().Lookup(flag)
		assert.NotNil(t, f, "consolidate flag --%s should be registered", flag)
	}
}

func TestSpotFlagsRegistered(t *testing.T) {
	root := newRootCmd()
	var spotCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "spot" {
			spotCmd = cmd
			break
		}
	}
	require.NotNil(t, spotCmd)

	flags := map[string]string{
		"min-replicas":      "2",
		"discount":          "0.65",
		"exclude-namespace": "kube-system",
		"controller-types":  "Deployment,ReplicaSet",
	}
	for flag, defaultVal := range flags {
		f := spotCmd.Flags().Lookup(flag)
		assert.NotNil(t, f, "spot flag --%s should be registered", flag)
		assert.Equal(t, defaultVal, f.DefValue, "spot flag --%s default", flag)
	}
}

func TestApplyFlagsRegistered(t *testing.T) {
	root := newRootCmd()
	var applyCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "apply" {
			applyCmd = cmd
			break
		}
	}
	require.NotNil(t, applyCmd)

	f := applyCmd.Flags().Lookup("file")
	assert.NotNil(t, f)
	assert.Equal(t, "f", f.Shorthand)
}

func TestCompareFlagsRegistered(t *testing.T) {
	root := newRootCmd()
	var compareCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "compare" {
			compareCmd = cmd
			break
		}
	}
	require.NotNil(t, compareCmd)

	f := compareCmd.Flags().Lookup("file")
	assert.NotNil(t, f)
	assert.Equal(t, "f", f.Shorthand)
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"default,api", []string{"default", "api"}},
		{"default, api, data-pipeline", []string{"default", "api", "data-pipeline"}},
		{"single", []string{"single"}},
		{"", nil},
		{" , , ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, splitCSV(tt.input))
		})
	}
}

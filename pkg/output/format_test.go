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

package output

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderDispatchTable(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()

	err := Render(&buf, report, "table")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "KubeWise")
}

func TestRenderDispatchJSON(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()

	err := Render(&buf, report, "json")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"scenario"`)
}

func TestRenderDispatchMarkdown(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()

	err := Render(&buf, report, "markdown")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "## KubeWise")
}

func TestRenderDispatchEmpty(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()

	// Empty string defaults to table
	err := Render(&buf, report, "")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "KubeWise")
}

func TestRenderDispatchUnknown(t *testing.T) {
	var buf bytes.Buffer
	report := newTestReport()

	err := Render(&buf, report, "xml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown output format")
	assert.Contains(t, err.Error(), "xml")
}

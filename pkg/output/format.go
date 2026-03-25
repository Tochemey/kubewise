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
	"fmt"
	"io"
)

// Render dispatches to the appropriate renderer based on the format string.
// Supported formats: "table", "json", "markdown".
func Render(w io.Writer, report Report, format string) error {
	switch format {
	case "table", "":
		return RenderTable(w, report)
	case "json":
		return RenderJSON(w, report)
	case "markdown":
		return RenderMarkdown(w, report)
	default:
		return fmt.Errorf("unknown output format: %q (supported: table, json, markdown)", format)
	}
}

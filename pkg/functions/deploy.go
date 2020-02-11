// Copyright © 2020 The Knative Authors
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

package functions

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"knative.dev/client/pkg/functions/sdks"
	"knative.dev/client/pkg/functions/template"
)

func RunDeploy(w io.Writer, sdk *SdkStatus) error {
	deployFile := fmt.Sprintf("%s%s", sdk.Dir, "deploy.yaml")
	deployPlan, err := sdks.LoadSDKInit(deployFile)
	if err != nil {
		return err
	}

	data := make(map[string]interface{})
	// TODO - this is a copy of the init code.
	// For prototype purposes, it's the same spec. Needs some thought.
	fmt.Fprintf(w, "Using SDK: %s deploy plans\n", sdk.SdkName)
	for _, step := range deployPlan.Spec.Steps {
		fmt.Fprintf(w, " ♫ %s\n", step.Name)
		if step.Mkdir != "" {
			createDir, err := template.InterpretString(step.Mkdir, data)
			err = os.MkdirAll(createDir, os.ModePerm)
			if err != nil {
				return err
			}
		} else if step.Exec != "" {
			parts := strings.Split(step.Exec, " ")
			path, err := exec.LookPath(parts[0])
			if err != nil {
				return err
			}
			args := make([]string, 0)
			if len(parts) > 0 {
				args = parts[1:]
			}

			cmd := exec.Command(path, args...)
			cmd.Env = os.Environ()
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return err
			}
		} else if step.File.Source != "" {
			sourceFile := fmt.Sprintf("%s/%s", sdk.Dir, step.File.Source)
			outFile, err := template.InterpretString(step.File.Destination, data)
			if err != nil {
				return err
			}
			err = template.RenderTemplate(sourceFile, outFile, data)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("invalid config step '%s' - no command specified", step.Name)
		}
	}

	return nil
}
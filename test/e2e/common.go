// Copyright 2019 The Knative Authors

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type runOpts struct {
	NoNamespace  bool
	AllowError   bool
	StderrWriter io.Writer
	StdoutWriter io.Writer
	StdinReader  io.Reader
	CancelCh     chan struct{}
	Redact       bool
}

const (
	KnDefaultTestImage string        = "gcr.io/knative-samples/helloworld-go"
	MaxRetries         int           = 10
	RetrySleepDuration time.Duration = 5 * time.Second
)

var m sync.Mutex

type e2eTest struct {
	env                    env
	kn                     kn
	createNamespaceOnSetup bool
	namespaceCreated       bool
}

func NewE2eTest(t *testing.T) *e2eTest {
	return &e2eTest{
		env:                    buildEnv(t),
		createNamespaceOnSetup: true,
	}
}

// Setup set up an environment for kn integration test returns the Teardown cleanup function
func (test *e2eTest) Setup(t *testing.T) {
	test.env.Namespace = fmt.Sprintf("%s%d", test.env.Namespace, getNamespaceCountAndIncrement())
	test.kn = kn{t, test.env.Namespace, Logger{}}
	if test.createNamespaceOnSetup {
		test.CreateTestNamespace(t, test.env.Namespace)
		test.WaitForNamespaceCreated(t, test.env.Namespace)
	}
}

func getNamespaceCountAndIncrement() int {
	m.Lock()
	defer m.Unlock()
	current := namespaceCount
	namespaceCount++
	return current
}

func getServiceNameAndIncrement(base string) string {
	m.Lock()
	defer m.Unlock()
	current := serviceCount
	serviceCount++
	return base + strconv.Itoa(current)
}

// Teardown clean up
func (test *e2eTest) Teardown(t *testing.T) {
	if test.namespaceCreated {
		test.DeleteTestNamespace(t, test.env.Namespace)
	}
}

// CreateTestNamespace creates and tests a namesspace creation invoking kubectl
func (test *e2eTest) CreateTestNamespace(t *testing.T, namespace string) {
	logger := Logger{}
	expectedOutputRegexp := fmt.Sprintf("namespace?.+%s.+created", namespace)
	out, err := createNamespace(t, namespace, MaxRetries, logger)
	if err != nil {
		logger.Fatalf("Could not create namespace with error %v, giving up\n", err)
	}

	// check that last output indeed show created namespace
	if !matchRegexp(t, expectedOutputRegexp, out) {
		t.Fatalf("Expected output incorrect, expecting to include:\n%s\n Instead found:\n%s\n", expectedOutputRegexp, out)
	}
	test.namespaceCreated = true
}

// CreateTestNamespace deletes and tests a namesspace deletion invoking kubectl
func (test *e2eTest) DeleteTestNamespace(t *testing.T, namespace string) {
	kubectl := kubectl{t, Logger{}}
	out, err := kubectl.RunWithOpts([]string{"delete", "namespace", namespace}, runOpts{})
	if err != nil {
		t.Fatalf("Error executing 'kubectl delete namespace' command. Error: %s", err.Error())
	}

	expectedOutputRegexp := fmt.Sprintf("namespace?.+%s.+deleted", namespace)
	if !matchRegexp(t, expectedOutputRegexp, out) {
		t.Fatalf("Expected output incorrect, expecting to include:\n%s\n Instead found:\n%s\n", expectedOutputRegexp, out)
	}
	test.namespaceCreated = false
}

// WaitForNamespaceDeleted wait until namespace is deleted
func (test *e2eTest) WaitForNamespaceDeleted(t *testing.T, namespace string) {
	logger := Logger{}
	deleted := checkNamespace(t, namespace, false, MaxRetries, logger)
	if !deleted {
		t.Fatalf("Error deleting namespace %s, timed out", namespace)
	}
}

// WaitForNamespaceCreated wait until namespace is created
func (test *e2eTest) WaitForNamespaceCreated(t *testing.T, namespace string) {
	logger := Logger{}
	created := checkNamespace(t, namespace, true, MaxRetries, logger)
	if !created {
		t.Fatalf("Error creating namespace %s, timed out", namespace)
	}
}

// Private functions
func checkNamespace(t *testing.T, namespace string, created bool, maxRetries int, logger Logger) bool {
	kubectlGetNamespace := func() (string, error) {
		kubectl := kubectl{t, logger}
		return kubectl.RunWithOpts([]string{"get", "namespace"}, runOpts{})
	}

	retries := 0
	for retries < MaxRetries {
		output, _ := kubectlGetNamespace()

		// check for namespace deleted
		if !created && !strings.Contains(output, namespace) {
			return true
		}

		// check for namespace created
		if created && strings.Contains(output, namespace) {
			return true
		}

		retries++
		logger.Debugf("Namespace is terminating, waiting %ds, and trying again: %d of %d\n", int(RetrySleepDuration.Seconds()), retries, maxRetries)
		time.Sleep(RetrySleepDuration)
	}

	return true
}

func createNamespace(t *testing.T, namespace string, maxRetries int, logger Logger) (string, error) {
	kubectlCreateNamespace := func() (string, error) {
		kubectl := kubectl{t, logger}
		return kubectl.RunWithOpts([]string{"create", "namespace", namespace}, runOpts{AllowError: true})
	}

	var (
		retries int
		err     error
		out     string
	)

	for retries < maxRetries {
		out, err = kubectlCreateNamespace()
		if err == nil {
			return out, nil
		}
		retries++
		logger.Debugf("Could not create namespace with error %v, waiting %ds, and trying again: %d of %d\n", err, int(RetrySleepDuration.Seconds()), retries, maxRetries)
		time.Sleep(RetrySleepDuration)
	}

	return out, err
}

func runCLIWithOpts(cli string, args []string, opts runOpts, logger Logger) (string, error) {
	logger.Debugf("Running '%s'...\n", cmdCLIDesc(cli, args))

	var stderr bytes.Buffer
	var stdout bytes.Buffer

	cmd := exec.Command(cli, args...)
	cmd.Stderr = &stderr

	if opts.CancelCh != nil {
		go func() {
			select {
			case <-opts.CancelCh:
				cmd.Process.Signal(os.Interrupt)
			}
		}()
	}

	if opts.StdoutWriter != nil {
		cmd.Stdout = opts.StdoutWriter
	} else {
		cmd.Stdout = &stdout
	}

	cmd.Stdin = opts.StdinReader

	err := cmd.Run()
	if err != nil {
		err = fmt.Errorf("Execution error: stderr: '%s' error: '%s'", stderr.String(), err)

		if !opts.AllowError {
			logger.Fatalf("Failed to successfully execute '%s': %v", cmdCLIDesc(cli, args), err)
		}
	}

	return stdout.String(), err
}

func cmdCLIDesc(cli string, args []string) string {
	return fmt.Sprintf("%s %s", cli, strings.Join(args, " "))
}

func matchRegexp(t *testing.T, matchingRegexp, actual string) bool {
	matched, err := regexp.MatchString(matchingRegexp, actual)
	if err != nil {
		t.Fatalf("Failed to match regexp '%s'. Error: '%s'", matchingRegexp, err.Error())
	}
	return matched
}

func currentDir(t *testing.T) string {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal("Unable to read current dir:", err)
	}
	return dir
}

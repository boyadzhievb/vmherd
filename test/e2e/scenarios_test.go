//go:build integration

package e2e

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/boyadzhievb/vmherd/internal/config"
)

const (
	// scenariosEnableVariable must be "1" to boot the acceptance scenarios;
	// they download packages in the guests and take about half an hour.
	scenariosEnableVariable = "VMHERD_E2E_SCENARIOS"
	// scenarioDirectoryVariable overrides where the scenario directories are,
	// for a cross-compiled e2e.test on a host without the source tree.
	scenarioDirectoryVariable = "VMHERD_E2E_SCENARIO_DIR"
	// defaultScenarioDirectory is test/acceptance, relative to this package.
	defaultScenarioDirectory = "../acceptance"
	// scenarioProjectPrefix starts every scenario's temporary project name,
	// so its domain prefix differs from any user project on the runner.
	scenarioProjectPrefix = "acceptance-"
	// applicationReadyTimeout bounds how long the HTTP checks retry while the
	// application's containers start.
	applicationReadyTimeout = 3 * time.Minute
	// httpRequestTimeout bounds one HTTP request of those checks.
	httpRequestTimeout = 10 * time.Second
	// httpInitialRetryInterval is the first wait between HTTP attempts.
	httpInitialRetryInterval = 1 * time.Second
	// httpMaxRetryInterval caps the exponential backoff between HTTP attempts.
	httpMaxRetryInterval = 8 * time.Second
	// responseExcerptLength is how much of a failed response body is shown.
	responseExcerptLength = 200
	// scenarioMarkerPath is written into every VM before down and must
	// still be there after up.
	scenarioMarkerPath = "/var/tmp/acceptance-marker"
	// scenarioMarkerContent is what scenarioMarkerPath holds.
	scenarioMarkerContent = "kept"
)

// nameResolutionCheck is a host name one VM must resolve to an address in
// the scenario's subnet.
type nameResolutionCheck struct {
	vmName   string // VM that runs the lookup
	hostName string // name it must resolve, e.g. redis.local
}

// acceptanceScenario is one directory under test/acceptance and what a user
// expects from it. Network, VMs and ports come from its Herdfile.
type acceptanceScenario struct {
	directoryName        string                // directory under test/acceptance
	applicationURLs      []string              // URLs that must answer HTTP 200 through forwarded ports
	nameResolutionChecks []nameResolutionCheck // lookups across VMs (multi-VM scenarios)
}

// acceptanceScenarios lists every scenario. They run one after the other:
// they forward the same host ports.
var acceptanceScenarios = []acceptanceScenario{
	{
		directoryName:   "terramino-single",
		applicationURLs: []string{"http://127.0.0.1:8081/", "http://127.0.0.1:8080/api/scores"},
	},
	{
		directoryName:   "terramino-multi",
		applicationURLs: []string{"http://127.0.0.1:8081/", "http://127.0.0.1:8080/api/scores"},
		nameResolutionChecks: []nameResolutionCheck{
			{vmName: "backend", hostName: "redis.local"},
			{vmName: "frontend", hostName: "backend.local"},
		},
	},
}

// TestAcceptanceScenarios boots every scenario in test/acceptance the way a
// user runs it and checks the application through its forwarded ports.
func TestAcceptanceScenarios(t *testing.T) {
	if os.Getenv(scenariosEnableVariable) != "1" {
		t.Skipf("set %s=1 to boot the acceptance scenarios (they download packages in the guests and take about 30 minutes)",
			scenariosEnableVariable)
	}
	scenarioRoot := os.Getenv(scenarioDirectoryVariable)
	if scenarioRoot == "" {
		scenarioRoot = defaultScenarioDirectory
	}
	for _, scenario := range acceptanceScenarios {
		t.Run(scenario.directoryName, func(t *testing.T) {
			runAcceptanceScenario(t, scenario, filepath.Join(scenarioRoot, scenario.directoryName))
		})
	}
}

// runAcceptanceScenario is the user's path through one scenario; each phase
// is a helper so a failure names the phase.
func runAcceptanceScenario(t *testing.T, scenario acceptanceScenario, sourceDirectory string) {
	hostPorts := forwardedHostPorts(loadHerdfile(t, sourceDirectory))
	checkHostPortsFree(t, hostPorts)
	project, herdfile := newScenarioProject(t, scenario, sourceDirectory)

	project.mustVMHerdLong(t, "up")
	checkForwardedPorts(t, scenario.applicationURLs)
	checkNamesResolveAcrossVMs(t, project, scenario.nameResolutionChecks, herdfile.Network.Subnet)
	checkProvisionRerun(t, project, scenario.applicationURLs)
	checkDownUpKeepsData(t, project, herdfile.VMNames(), scenario.applicationURLs)
	checkDestroyLeavesNothing(t, project)
	checkHostPortsFree(t, hostPorts)
}

// newScenarioProject copies the scenario into a fresh project directory,
// creates the share directories its Herdfile names, and removes leftovers of
// an earlier, aborted run.
func newScenarioProject(t *testing.T, scenario acceptanceScenario, sourceDirectory string) (e2eProject, *config.Herdfile) {
	t.Helper()
	projectName := scenarioProjectPrefix + scenario.directoryName
	projectDirectory := filepath.Join(t.TempDir(), projectName)
	if err := copyDirectory(sourceDirectory, projectDirectory); err != nil {
		t.Fatalf("copying scenario %s: %v", scenario.directoryName, err)
	}
	herdfile := loadHerdfile(t, projectDirectory)
	for _, vmConfig := range herdfile.VMs {
		for _, sharedFolder := range vmConfig.SharedFolders {
			if err := os.MkdirAll(sharedFolder.ResolvedHost, 0o755); err != nil {
				t.Fatalf("creating share directory %s: %v", sharedFolder.ResolvedHost, err)
			}
		}
	}
	project := e2eProject{
		directory:    projectDirectory,
		stateDir:     filepath.Join(t.TempDir(), "state"),
		domainPrefix: "vmherd-" + projectName + "-",
		networkName:  herdfile.Network.Name,
	}
	cleanUpNowAndOnFailure(t, project)
	return project, herdfile
}

// loadHerdfile loads the Herdfile in directory and fails the test on any problem.
func loadHerdfile(t *testing.T, directory string) *config.Herdfile {
	t.Helper()
	absoluteDirectory, err := filepath.Abs(directory)
	if err != nil {
		t.Fatalf("resolving %s: %v", directory, err)
	}
	herdfile, err := config.Load(absoluteDirectory)
	if err != nil {
		t.Fatalf("loading the Herdfile in %s: %v", directory, err)
	}
	return herdfile
}

// copyDirectory copies the regular files and directories under source to
// destination, keeping file modes. Anything else (symlinks) is an error.
func copyDirectory(source, destination string) error {
	return filepath.WalkDir(source, func(sourcePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(source, sourcePath)
		if err != nil {
			return err
		}
		destinationPath := filepath.Join(destination, relativePath)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			return os.MkdirAll(destinationPath, 0o755)
		case info.Mode().IsRegular():
			content, err := os.ReadFile(sourcePath)
			if err != nil {
				return err
			}
			return os.WriteFile(destinationPath, content, info.Mode().Perm())
		default:
			return fmt.Errorf("%s is neither a file nor a directory — scenarios hold only plain files", sourcePath)
		}
	})
}

// forwardedHostPorts lists every host port the Herdfile forwards.
func forwardedHostPorts(herdfile *config.Herdfile) []int {
	var hostPorts []int
	for _, vmConfig := range herdfile.VMs {
		for _, portForward := range vmConfig.Ports {
			hostPorts = append(hostPorts, portForward.Host)
		}
	}
	return hostPorts
}

// checkHostPortsFree fails the test, naming the port and the process holding
// it, when any host port the scenario forwards can't be listened on.
func checkHostPortsFree(t *testing.T, hostPorts []int) {
	t.Helper()
	for _, hostPort := range hostPorts {
		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(hostPort)))
		if err != nil {
			t.Fatalf("host port %d is not free: %v\n%s", hostPort, err, describePortHolder(t, hostPort))
			return
		}
		if err := listener.Close(); err != nil {
			t.Fatalf("closing test listener on port %d: %v", hostPort, err)
		}
	}
}

// describePortHolder returns what 'ss' reports about the listener on
// hostPort, or why it couldn't be asked.
func describePortHolder(t *testing.T, hostPort int) string {
	t.Helper()
	commandContext, cancel := context.WithTimeout(context.Background(), shortCommandTimeout)
	defer cancel()
	output, err := exec.CommandContext(commandContext, "sudo", "ss", "-ltnp", fmt.Sprintf("sport = :%d", hostPort)).CombinedOutput()
	if err != nil {
		return fmt.Sprintf("(cannot ask ss who holds it: %v: %s)", err, strings.TrimSpace(string(output)))
	}
	return "held by:\n" + strings.TrimSpace(string(output))
}

// checkForwardedPorts fails the test unless every URL answers HTTP 200
// within applicationReadyTimeout, naming each URL and port that doesn't.
func checkForwardedPorts(t *testing.T, applicationURLs []string) {
	t.Helper()
	var failures []error
	for _, applicationURL := range applicationURLs {
		if err := waitForHTTPStatusOK(applicationURL); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		t.Fatalf("application not reachable through forwarded ports:\n%v", errors.Join(failures...))
	}
}

// waitForHTTPStatusOK requests applicationURL with exponential backoff until
// it answers 200, and returns an error naming the URL, port and last outcome
// after applicationReadyTimeout.
func waitForHTTPStatusOK(applicationURL string) error {
	parsedURL, err := url.Parse(applicationURL)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", applicationURL, err)
	}
	httpClient := &http.Client{Timeout: httpRequestTimeout}
	deadline := time.Now().Add(applicationReadyTimeout)
	retryInterval := httpInitialRetryInterval
	lastOutcome := "no request made"
	for time.Now().Before(deadline) {
		statusCode, bodyExcerpt, requestErr := fetchURL(httpClient, applicationURL)
		switch {
		case requestErr != nil:
			lastOutcome = requestErr.Error()
		case statusCode == http.StatusOK:
			return nil
		default:
			lastOutcome = fmt.Sprintf("HTTP %d: %q", statusCode, bodyExcerpt)
		}
		time.Sleep(retryInterval)
		retryInterval = min(2*retryInterval, httpMaxRetryInterval)
	}
	return fmt.Errorf("  %s (port %s): no HTTP 200 within %s; last: %s", applicationURL, parsedURL.Port(), applicationReadyTimeout, lastOutcome)
}

// fetchURL makes one GET request and returns the status code and the start
// of the body.
func fetchURL(httpClient *http.Client, applicationURL string) (int, string, error) {
	response, err := httpClient.Get(applicationURL) //nolint:noctx // httpClient.Timeout bounds the request
	if err != nil {
		return 0, "", err
	}
	bodyExcerpt, readErr := io.ReadAll(io.LimitReader(response.Body, responseExcerptLength))
	closeErr := response.Body.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return 0, "", fmt.Errorf("reading the response from %s: %w", applicationURL, err)
	}
	return response.StatusCode, string(bodyExcerpt), nil
}

// checkNamesResolveAcrossVMs runs each lookup in its VM and checks every
// address it prints is inside the scenario's subnet.
func checkNamesResolveAcrossVMs(t *testing.T, project e2eProject, checks []nameResolutionCheck, subnetCIDR string) {
	t.Helper()
	_, subnet, err := net.ParseCIDR(subnetCIDR)
	if err != nil {
		t.Fatalf("parsing subnet %s: %v", subnetCIDR, err)
	}
	for _, check := range checks {
		output := strings.TrimSpace(project.mustVMHerd(t, shortCommandTimeout, "ssh", check.vmName, "--", "getent", "hosts", check.hostName))
		if output == "" {
			t.Errorf("%s: getent hosts %s printed nothing", check.vmName, check.hostName)
			continue
		}
		for _, line := range strings.Split(output, "\n") {
			address := net.ParseIP(strings.Fields(line)[0])
			if address == nil || !subnet.Contains(address) {
				t.Errorf("%s: %s resolves to %q, want an address in %s", check.vmName, check.hostName, line, subnetCIDR)
			}
		}
	}
}

// checkProvisionRerun reruns every VM's provisioning and checks the
// application still answers afterwards.
func checkProvisionRerun(t *testing.T, project e2eProject, applicationURLs []string) {
	t.Helper()
	project.mustVMHerdLong(t, "provision")
	checkForwardedPorts(t, applicationURLs)
}

// checkDownUpKeepsData writes a marker into every VM, stops and starts the
// project, and checks the markers survived and the application answers again.
func checkDownUpKeepsData(t *testing.T, project e2eProject, vmNames []string, applicationURLs []string) {
	t.Helper()
	writeMarkerCommand := fmt.Sprintf("echo %s > %s", scenarioMarkerContent, scenarioMarkerPath)
	for _, vmName := range vmNames {
		// One argument is sent as a raw command line, so the redirect runs in the guest.
		project.mustVMHerd(t, shortCommandTimeout, "ssh", vmName, "--", writeMarkerCommand)
	}
	project.mustVMHerd(t, shortCommandTimeout, "down")
	project.mustVMHerdLong(t, "up")
	for _, vmName := range vmNames {
		markerContent := project.mustVMHerd(t, shortCommandTimeout, "ssh", vmName, "--", "cat", scenarioMarkerPath)
		if strings.TrimSpace(markerContent) != scenarioMarkerContent {
			t.Errorf("%s: %s after down + up holds %q, want %q", vmName, scenarioMarkerPath, markerContent, scenarioMarkerContent)
		}
	}
	checkForwardedPorts(t, applicationURLs)
}

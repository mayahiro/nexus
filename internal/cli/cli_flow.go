package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mayahiro/nexus/internal/api"
	comparecmd "github.com/mayahiro/nexus/internal/cli/compare"
	"github.com/mayahiro/nexus/internal/config"
	"github.com/mayahiro/nexus/internal/rpc"
)

type flowManifest struct {
	Defaults  flowDefaults          `json:"defaults,omitempty"`
	Matrices  map[string]flowMatrix `json:"matrices,omitempty"`
	Scenarios []flowScenario        `json:"scenarios,omitempty"`
}

type flowDefaults struct {
	Backend         string   `json:"backend,omitempty"`
	TargetRef       string   `json:"target_ref,omitempty"`
	Viewport        string   `json:"viewport,omitempty"`
	WaitTimeout     *int     `json:"wait_timeout,omitempty"`
	CompareCSS      bool     `json:"compare_css,omitempty"`
	CSSProperty     []string `json:"css_property,omitempty"`
	IgnoreTextRegex []string `json:"ignore_text_regex,omitempty"`
	IgnoreSelector  []string `json:"ignore_selector,omitempty"`
	MaskSelector    []string `json:"mask_selector,omitempty"`
}

type flowMatrix struct {
	Backend   string            `json:"backend,omitempty"`
	TargetRef string            `json:"target_ref,omitempty"`
	Viewport  string            `json:"viewport,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
}

type flowScenario struct {
	Name      string            `json:"name,omitempty"`
	Matrix    []string          `json:"matrix,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
	Old       flowEndpoint      `json:"old"`
	New       flowEndpoint      `json:"new"`
	Steps     []flowStep        `json:"steps,omitempty"`
}

type flowEndpoint struct {
	URL       string `json:"url,omitempty"`
	Session   string `json:"session,omitempty"`
	Backend   string `json:"backend,omitempty"`
	TargetRef string `json:"target_ref,omitempty"`
	Viewport  string `json:"viewport,omitempty"`
}

type flowStep struct {
	Name            string   `json:"name,omitempty"`
	Side            string   `json:"side,omitempty"`
	Action          string   `json:"action,omitempty"`
	Locator         string   `json:"locator,omitempty"`
	Nth             int      `json:"nth,omitempty"`
	Text            string   `json:"text,omitempty"`
	Target          string   `json:"target,omitempty"`
	Value           string   `json:"value,omitempty"`
	Path            string   `json:"path,omitempty"`
	State           string   `json:"state,omitempty"`
	Timeout         *int     `json:"timeout,omitempty"`
	ContinueOnError bool     `json:"continue_on_error,omitempty"`
	Full            bool     `json:"full,omitempty"`
	Annotate        bool     `json:"annotate,omitempty"`
	CompareCSS      *bool    `json:"compare_css,omitempty"`
	CSSProperty     []string `json:"css_property,omitempty"`
	IgnoreTextRegex []string `json:"ignore_text_regex,omitempty"`
	IgnoreSelector  []string `json:"ignore_selector,omitempty"`
	MaskSelector    []string `json:"mask_selector,omitempty"`
}

type flowResolvedEndpoint struct {
	URL       string
	SessionID string
	Backend   string
	TargetRef string
	Viewport  string
}

type flowExpandedScenario struct {
	Name      string
	Matrix    string
	Variables map[string]string
	Old       flowResolvedEndpoint
	New       flowResolvedEndpoint
	Steps     []flowStep
}

type flowPreparedSession struct {
	SessionID string
	Detach    bool
}

type flowReport struct {
	Manifest  string               `json:"manifest,omitempty"`
	Summary   flowReportSummary    `json:"summary"`
	Scenarios []flowScenarioReport `json:"scenarios"`
}

type flowReportSummary struct {
	TotalScenarios     int `json:"total_scenarios"`
	CompletedScenarios int `json:"completed_scenarios"`
	FailedScenarios    int `json:"failed_scenarios"`
	TotalSteps         int `json:"total_steps"`
	FailedSteps        int `json:"failed_steps"`
	TotalCompares      int `json:"total_compares"`
	DifferentCompares  int `json:"different_compares"`
}

type flowScenarioReport struct {
	Name   string           `json:"name"`
	Matrix string           `json:"matrix,omitempty"`
	Status string           `json:"status"`
	Error  string           `json:"error,omitempty"`
	Steps  []flowStepReport `json:"steps,omitempty"`
}

type flowStepReport struct {
	Name        string            `json:"name,omitempty"`
	Action      string            `json:"action"`
	Side        string            `json:"side,omitempty"`
	Status      string            `json:"status"`
	Error       string            `json:"error,omitempty"`
	Compare     json.RawMessage   `json:"compare,omitempty"`
	Screenshots map[string]string `json:"screenshots,omitempty"`
}

type flowCompareSummary struct {
	Same          bool `json:"same"`
	TotalFindings int  `json:"total_findings"`
}

type flowCompareReport struct {
	Summary flowCompareSummary `json:"summary"`
}

type flowSelectorTerm struct {
	Kind  string
	Value string
}

func runFlow(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printFlowHelp(stdout)
		return 0
	}
	if len(args) == 0 {
		printFlowHelp(stderr)
		return 1
	}

	switch args[0] {
	case "run":
		return runFlowRun(ctx, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown flow subcommand: %s\n", args[0])
		printFlowHelp(stderr)
		return 1
	}
}

func runFlowRun(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if isHelpArgs(args) {
		printFlowRunHelp(stdout)
		return 0
	}
	fs := flag.NewFlagSet("flow run", flag.ContinueOnError)
	fs.SetOutput(stderr)

	manifestPath := fs.String("manifest", "", "flow manifest json")
	scenarioName := fs.String("scenario", "", "scenario name")
	matrixName := fs.String("matrix", "", "matrix name")
	continueOnError := fs.Bool("continue-on-error", false, "continue after scenario error")
	asJSON := fs.Bool("json", false, "print as json")
	outputJSON := fs.String("output-json", "", "write flow report json to file")

	if err := parseCommandFlags(fs, args, stderr, "flow"); err != nil {
		return 1
	}
	if strings.TrimSpace(*manifestPath) == "" {
		fmt.Fprintln(stderr, "flow run requires --manifest")
		fmt.Fprintln(stderr, "hint: nxctl flow run --manifest login-flow.json")
		return 1
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "flow run does not accept positional arguments")
		return 1
	}

	manifest, err := loadFlowManifest(*manifestPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	expanded, err := expandFlowManifest(manifest, strings.TrimSpace(*scenarioName), strings.TrimSpace(*matrixName))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	report := flowReport{
		Manifest:  *manifestPath,
		Scenarios: make([]flowScenarioReport, 0, len(expanded)),
	}

	for _, scenario := range expanded {
		scenarioReport, err := executeFlowScenario(ctx, client, paths, manifest.Defaults, scenario)
		if err != nil {
			scenarioReport.Status = "failed"
			scenarioReport.Error = err.Error()
			report.Scenarios = append(report.Scenarios, scenarioReport)
			if !*continueOnError {
				report.Summary = summarizeFlowReport(report.Scenarios)
				if strings.TrimSpace(*outputJSON) != "" {
					if err := writeFlowJSONFile(*outputJSON, report); err != nil {
						fmt.Fprintln(stderr, err)
						return 1
					}
				}
				if *asJSON {
					encoder := json.NewEncoder(stdout)
					encoder.SetIndent("", "  ")
					if err := encoder.Encode(report); err != nil {
						fmt.Fprintln(stderr, err)
						return 1
					}
					return 1
				}
				printFlowReport(stdout, report)
				return 1
			}
			continue
		}
		report.Scenarios = append(report.Scenarios, scenarioReport)
	}

	report.Summary = summarizeFlowReport(report.Scenarios)
	if strings.TrimSpace(*outputJSON) != "" {
		if err := writeFlowJSONFile(*outputJSON, report); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if report.Summary.FailedScenarios > 0 {
			return 1
		}
		return 0
	}

	printFlowReport(stdout, report)
	if report.Summary.FailedScenarios > 0 {
		return 1
	}
	return 0
}

func loadFlowManifest(path string) (flowManifest, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return flowManifest{}, err
	}
	var manifest flowManifest
	if err := json.Unmarshal(bytes, &manifest); err != nil {
		return flowManifest{}, fmt.Errorf("invalid flow manifest %q: %w", path, err)
	}
	if len(manifest.Scenarios) == 0 {
		return flowManifest{}, fmt.Errorf("flow manifest %q requires at least one scenario", path)
	}
	return manifest, nil
}

func expandFlowManifest(manifest flowManifest, scenarioFilter string, matrixFilter string) ([]flowExpandedScenario, error) {
	expanded := make([]flowExpandedScenario, 0, len(manifest.Scenarios))
	foundScenario := scenarioFilter == ""
	foundMatrix := matrixFilter == ""

	for i, scenario := range manifest.Scenarios {
		name := strings.TrimSpace(scenario.Name)
		if name == "" {
			name = fmt.Sprintf("scenario[%d]", i)
		}
		if scenarioFilter != "" && name != scenarioFilter {
			continue
		}
		foundScenario = true

		matrixNames := scenario.Matrix
		if len(matrixNames) == 0 {
			matrixNames = []string{""}
		}

		for _, matrixName := range matrixNames {
			matrixName = strings.TrimSpace(matrixName)
			if matrixFilter != "" && matrixName != matrixFilter {
				continue
			}
			if matrixFilter != "" {
				foundMatrix = true
			}

			matrix := flowMatrix{}
			if matrixName != "" {
				value, ok := manifest.Matrices[matrixName]
				if !ok {
					return nil, fmt.Errorf("scenario %s references unknown matrix %q", name, matrixName)
				}
				matrix = value
			}

			vars := make(map[string]string, len(matrix.Variables)+len(scenario.Variables))
			for key, value := range matrix.Variables {
				vars[key] = value
			}
			for key, value := range scenario.Variables {
				vars[key] = value
			}

			oldEndpoint, err := resolveFlowEndpoint(manifest.Defaults, matrix, scenario.Old, vars)
			if err != nil {
				return nil, fmt.Errorf("scenario %s old endpoint: %w", name, err)
			}
			newEndpoint, err := resolveFlowEndpoint(manifest.Defaults, matrix, scenario.New, vars)
			if err != nil {
				return nil, fmt.Errorf("scenario %s new endpoint: %w", name, err)
			}

			runName := name
			if matrixName != "" {
				runName = name + "[" + matrixName + "]"
			}

			steps := make([]flowStep, 0, len(scenario.Steps))
			for _, step := range scenario.Steps {
				resolved, err := resolveFlowStep(step, vars)
				if err != nil {
					return nil, fmt.Errorf("scenario %s step %q: %w", runName, flowStepName(step), err)
				}
				steps = append(steps, resolved)
			}

			expanded = append(expanded, flowExpandedScenario{
				Name:      runName,
				Matrix:    matrixName,
				Variables: vars,
				Old:       oldEndpoint,
				New:       newEndpoint,
				Steps:     steps,
			})
		}
	}

	if !foundScenario {
		return nil, fmt.Errorf("scenario %q not found", scenarioFilter)
	}
	if !foundMatrix {
		return nil, fmt.Errorf("matrix %q not found", matrixFilter)
	}
	if len(expanded) == 0 {
		return nil, errors.New("flow manifest produced no runnable scenarios")
	}
	return expanded, nil
}

func resolveFlowEndpoint(defaults flowDefaults, matrix flowMatrix, endpoint flowEndpoint, vars map[string]string) (flowResolvedEndpoint, error) {
	urlValue, err := expandFlowString(endpoint.URL, vars)
	if err != nil {
		return flowResolvedEndpoint{}, err
	}
	sessionValue, err := expandFlowString(endpoint.Session, vars)
	if err != nil {
		return flowResolvedEndpoint{}, err
	}
	backendValue, err := expandFlowString(endpoint.Backend, vars)
	if err != nil {
		return flowResolvedEndpoint{}, err
	}
	targetRefValue, err := expandFlowString(endpoint.TargetRef, vars)
	if err != nil {
		return flowResolvedEndpoint{}, err
	}
	viewportValue, err := expandFlowString(endpoint.Viewport, vars)
	if err != nil {
		return flowResolvedEndpoint{}, err
	}

	if strings.TrimSpace(backendValue) == "" {
		backendValue = strings.TrimSpace(matrix.Backend)
	}
	if strings.TrimSpace(backendValue) == "" {
		backendValue = strings.TrimSpace(defaults.Backend)
	}
	if strings.TrimSpace(backendValue) == "" {
		backendValue = "chromium"
	}

	if strings.TrimSpace(targetRefValue) == "" {
		targetRefValue = strings.TrimSpace(matrix.TargetRef)
	}
	if strings.TrimSpace(targetRefValue) == "" {
		targetRefValue = strings.TrimSpace(defaults.TargetRef)
	}

	if strings.TrimSpace(viewportValue) == "" {
		viewportValue = strings.TrimSpace(matrix.Viewport)
	}
	if strings.TrimSpace(viewportValue) == "" {
		viewportValue = strings.TrimSpace(defaults.Viewport)
	}

	switch {
	case strings.TrimSpace(urlValue) == "" && strings.TrimSpace(sessionValue) == "":
		return flowResolvedEndpoint{}, errors.New("endpoint requires url or session")
	case strings.TrimSpace(urlValue) != "" && strings.TrimSpace(sessionValue) != "":
		return flowResolvedEndpoint{}, errors.New("endpoint can not use both url and session")
	}

	return flowResolvedEndpoint{
		URL:       strings.TrimSpace(urlValue),
		SessionID: strings.TrimSpace(sessionValue),
		Backend:   strings.TrimSpace(backendValue),
		TargetRef: strings.TrimSpace(targetRefValue),
		Viewport:  strings.TrimSpace(viewportValue),
	}, nil
}

func resolveFlowStep(step flowStep, vars map[string]string) (flowStep, error) {
	var err error
	step.Name, err = expandFlowString(step.Name, vars)
	if err != nil {
		return flowStep{}, err
	}
	step.Locator, err = expandFlowString(step.Locator, vars)
	if err != nil {
		return flowStep{}, err
	}
	step.Text, err = expandFlowString(step.Text, vars)
	if err != nil {
		return flowStep{}, err
	}
	step.Target, err = expandFlowString(step.Target, vars)
	if err != nil {
		return flowStep{}, err
	}
	step.Value, err = expandFlowString(step.Value, vars)
	if err != nil {
		return flowStep{}, err
	}
	step.Path, err = expandFlowString(step.Path, vars)
	if err != nil {
		return flowStep{}, err
	}
	step.State, err = expandFlowString(step.State, vars)
	if err != nil {
		return flowStep{}, err
	}
	return step, nil
}

func expandFlowString(value string, vars map[string]string) (string, error) {
	value = strings.TrimSpace(value)
	for {
		start := strings.Index(value, "{{")
		if start < 0 {
			return value, nil
		}
		end := strings.Index(value[start+2:], "}}")
		if end < 0 {
			return "", errors.New("unterminated variable expression")
		}
		end += start + 2
		key := strings.TrimSpace(value[start+2 : end])
		replacement, ok := vars[key]
		if !ok {
			return "", fmt.Errorf("unknown variable %q", key)
		}
		value = value[:start] + replacement + value[end+2:]
	}
}

func executeFlowScenario(ctx context.Context, client *rpc.Client, paths config.Paths, defaults flowDefaults, scenario flowExpandedScenario) (flowScenarioReport, error) {
	report := flowScenarioReport{
		Name:   scenario.Name,
		Matrix: scenario.Matrix,
		Status: "completed",
		Steps:  make([]flowStepReport, 0, len(scenario.Steps)),
	}

	oldPrepared, err := prepareFlowSession(ctx, client, paths, "old", scenario.Old)
	if err != nil {
		return report, err
	}
	defer cleanupFlowSession(context.Background(), client, oldPrepared)

	newPrepared, err := prepareFlowSession(ctx, client, paths, "new", scenario.New)
	if err != nil {
		return report, err
	}
	defer cleanupFlowSession(context.Background(), client, newPrepared)

	state := flowExecutionState{
		Defaults: defaults,
		Old:      oldPrepared,
		New:      newPrepared,
	}

	for _, step := range scenario.Steps {
		stepReport, err := executeFlowStep(ctx, client, state, step)
		report.Steps = append(report.Steps, stepReport)
		if err != nil {
			report.Status = "failed"
			report.Error = err.Error()
			return report, err
		}
	}

	return report, nil
}

type flowExecutionState struct {
	Defaults flowDefaults
	Old      flowPreparedSession
	New      flowPreparedSession
}

func prepareFlowSession(ctx context.Context, client *rpc.Client, paths config.Paths, label string, endpoint flowResolvedEndpoint) (flowPreparedSession, error) {
	if endpoint.SessionID != "" {
		prepared := flowPreparedSession{SessionID: endpoint.SessionID}
		if strings.TrimSpace(endpoint.Viewport) != "" {
			if err := applyFlowViewport(ctx, client, prepared.SessionID, endpoint.Viewport); err != nil {
				return flowPreparedSession{}, fmt.Errorf("%s viewport: %w", label, err)
			}
		}
		return prepared, nil
	}

	resolvedTargetRef := strings.TrimSpace(endpoint.TargetRef)
	if resolvedTargetRef == "" {
		installation, err := newBrowserManager(paths).Resolve(endpoint.Backend)
		if err != nil {
			return flowPreparedSession{}, err
		}
		resolvedTargetRef = installation.ExecutablePath
	}

	width, height, err := resolvedViewport(endpoint.Viewport)
	if err != nil {
		return flowPreparedSession{}, err
	}

	sessionID := fmt.Sprintf("flow-%s-%d", label, time.Now().UnixNano())
	res, err := client.AttachSession(ctx, api.AttachSessionRequest{
		TargetType: "browser",
		SessionID:  sessionID,
		TargetRef:  resolvedTargetRef,
		Backend:    endpoint.Backend,
		Options: map[string]string{
			"initial_url":     endpoint.URL,
			"viewport_width":  strconv.Itoa(width),
			"viewport_height": strconv.Itoa(height),
		},
	})
	if err != nil {
		return flowPreparedSession{}, err
	}

	return flowPreparedSession{
		SessionID: res.Session.ID,
		Detach:    true,
	}, nil
}

func cleanupFlowSession(ctx context.Context, client *rpc.Client, prepared flowPreparedSession) {
	if !prepared.Detach || prepared.SessionID == "" {
		return
	}
	detachCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, _ = client.DetachSession(detachCtx, api.DetachSessionRequest{SessionID: prepared.SessionID})
}

func executeFlowStep(ctx context.Context, client *rpc.Client, state flowExecutionState, step flowStep) (flowStepReport, error) {
	report := flowStepReport{
		Name:   flowStepName(step),
		Action: strings.TrimSpace(step.Action),
		Side:   flowStepSide(step),
		Status: "completed",
	}

	switch strings.TrimSpace(step.Action) {
	case "wait":
		err := executeFlowWaitStep(ctx, client, state, step)
		if err != nil {
			report.Status = "failed"
			report.Error = err.Error()
			if step.ContinueOnError {
				return report, nil
			}
			return report, err
		}
	case "navigate":
		err := executeFlowNavigateStep(ctx, client, state, step)
		if err != nil {
			report.Status = "failed"
			report.Error = err.Error()
			if step.ContinueOnError {
				return report, nil
			}
			return report, err
		}
	case "click":
		err := executeFlowNodeStep(ctx, client, state, step, "invoke")
		if err != nil {
			report.Status = "failed"
			report.Error = err.Error()
			if step.ContinueOnError {
				return report, nil
			}
			return report, err
		}
	case "fill":
		err := executeFlowNodeStep(ctx, client, state, step, "fill")
		if err != nil {
			report.Status = "failed"
			report.Error = err.Error()
			if step.ContinueOnError {
				return report, nil
			}
			return report, err
		}
	case "viewport":
		err := executeFlowViewportStep(ctx, client, state, step)
		if err != nil {
			report.Status = "failed"
			report.Error = err.Error()
			if step.ContinueOnError {
				return report, nil
			}
			return report, err
		}
	case "compare":
		raw, err := executeFlowCompareStep(ctx, state, step)
		report.Compare = raw
		if err != nil {
			report.Status = "failed"
			report.Error = err.Error()
			if step.ContinueOnError {
				return report, nil
			}
			return report, err
		}
	case "screenshot":
		paths, err := executeFlowScreenshotStep(ctx, client, state, step)
		report.Screenshots = paths
		if err != nil {
			report.Status = "failed"
			report.Error = err.Error()
			if step.ContinueOnError {
				return report, nil
			}
			return report, err
		}
	default:
		err := fmt.Errorf("unsupported flow action: %s", strings.TrimSpace(step.Action))
		report.Status = "failed"
		report.Error = err.Error()
		if step.ContinueOnError {
			return report, nil
		}
		return report, err
	}

	return report, nil
}

func executeFlowWaitStep(ctx context.Context, client *rpc.Client, state flowExecutionState, step flowStep) error {
	target := strings.TrimSpace(step.Target)
	if target == "" {
		return errors.New("wait step requires target")
	}
	args := map[string]string{
		"target": target,
	}
	if strings.TrimSpace(step.Value) != "" {
		args["value"] = strings.TrimSpace(step.Value)
	}
	if strings.TrimSpace(step.State) != "" {
		args["state"] = strings.TrimSpace(step.State)
	}
	if step.Timeout != nil {
		args["timeout_ms"] = strconv.Itoa(*step.Timeout)
	}
	return executeFlowActionOnSides(ctx, client, state, step, api.Action{Kind: "wait", Args: args})
}

func executeFlowNavigateStep(ctx context.Context, client *rpc.Client, state flowExecutionState, step flowStep) error {
	if strings.TrimSpace(step.Value) == "" {
		return errors.New("navigate step requires value")
	}
	return executeFlowActionOnSides(ctx, client, state, step, api.Action{
		Kind: "navigate",
		Args: map[string]string{
			"url": strings.TrimSpace(step.Value),
		},
	})
}

func executeFlowNodeStep(ctx context.Context, client *rpc.Client, state flowExecutionState, step flowStep, actionKind string) error {
	if strings.TrimSpace(step.Locator) == "" {
		return fmt.Errorf("%s step requires locator", actionKind)
	}
	if step.Nth < 0 {
		return errors.New("nth must be a positive integer")
	}
	if actionKind == "fill" && step.Text == "" {
		return errors.New("fill step requires text")
	}

	for _, sessionID := range flowStepSessions(state, step) {
		node, err := resolveFlowLocator(ctx, client, sessionID, step.Locator, nodeSelectionOptions{Nth: step.Nth})
		if err != nil {
			return err
		}
		action := api.Action{
			Kind:   actionKind,
			NodeID: &node.ID,
			Text:   step.Text,
		}
		res, err := client.ActSession(ctx, api.ActSessionRequest{
			SessionID: sessionID,
			Action:    action,
		})
		if err != nil {
			return err
		}
		if !res.Result.OK {
			if strings.TrimSpace(res.Result.Message) != "" {
				return errors.New(strings.TrimSpace(res.Result.Message))
			}
			return fmt.Errorf("%s failed", actionKind)
		}
	}
	return nil
}

func executeFlowViewportStep(ctx context.Context, client *rpc.Client, state flowExecutionState, step flowStep) error {
	if strings.TrimSpace(step.Value) == "" {
		return errors.New("viewport step requires value")
	}
	for _, sessionID := range flowStepSessions(state, step) {
		if err := applyFlowViewport(ctx, client, sessionID, step.Value); err != nil {
			return err
		}
	}
	return nil
}

func applyFlowViewport(ctx context.Context, client *rpc.Client, sessionID string, value string) error {
	width, height, err := parseViewport(value)
	if err != nil {
		return err
	}
	res, err := client.ActSession(ctx, api.ActSessionRequest{
		SessionID: sessionID,
		Action: api.Action{
			Kind: "viewport",
			Args: map[string]string{
				"width":  strconv.Itoa(width),
				"height": strconv.Itoa(height),
			},
		},
	})
	if err != nil {
		return err
	}
	if !res.Result.OK {
		if strings.TrimSpace(res.Result.Message) != "" {
			return errors.New(strings.TrimSpace(res.Result.Message))
		}
		return errors.New("viewport failed")
	}
	return nil
}

func executeFlowCompareStep(ctx context.Context, state flowExecutionState, step flowStep) (json.RawMessage, error) {
	args := []string{
		"--old-session", state.Old.SessionID,
		"--new-session", state.New.SessionID,
		"--json",
	}
	compareCSS := state.Defaults.CompareCSS
	if step.CompareCSS != nil {
		compareCSS = *step.CompareCSS
	}
	if compareCSS {
		args = append(args, "--compare-css")
	}
	cssProperties := state.Defaults.CSSProperty
	if len(step.CSSProperty) > 0 {
		cssProperties = append([]string(nil), step.CSSProperty...)
	}
	for _, value := range cssProperties {
		args = append(args, "--css-property", value)
	}
	ignoreTextRegex := append([]string(nil), state.Defaults.IgnoreTextRegex...)
	ignoreTextRegex = append(ignoreTextRegex, step.IgnoreTextRegex...)
	for _, value := range ignoreTextRegex {
		args = append(args, "--ignore-text-regex", value)
	}
	ignoreSelector := append([]string(nil), state.Defaults.IgnoreSelector...)
	ignoreSelector = append(ignoreSelector, step.IgnoreSelector...)
	for _, value := range ignoreSelector {
		args = append(args, "--ignore-selector", value)
	}
	maskSelector := append([]string(nil), state.Defaults.MaskSelector...)
	maskSelector = append(maskSelector, step.MaskSelector...)
	for _, value := range maskSelector {
		args = append(args, "--mask-selector", value)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := comparecmd.Run(ctx, args, &stdout, &stderr, connectClient)
	if code != 0 {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = "compare failed"
		}
		return nil, errors.New(message)
	}

	raw := json.RawMessage(append([]byte(nil), bytes.TrimSpace(stdout.Bytes())...))
	return raw, nil
}

func executeFlowScreenshotStep(ctx context.Context, client *rpc.Client, state flowExecutionState, step flowStep) (map[string]string, error) {
	basePath := strings.TrimSpace(step.Path)
	if basePath == "" {
		return nil, errors.New("screenshot step requires path")
	}
	if step.Nth < 0 {
		return nil, errors.New("nth must be a positive integer")
	}
	if strings.TrimSpace(step.Locator) != "" && step.Full {
		return nil, errors.New("full is not supported with screenshot locator")
	}

	targets := flowStepTargets(state, step)
	paths := make(map[string]string, len(targets))
	multi := len(targets) > 1

	for _, target := range targets {
		data, err := captureScreenshotBytes(ctx, client, target.SessionID, screenshotCaptureOptions{
			Annotate: step.Annotate,
			Full:     step.Full,
			Locator:  strings.TrimSpace(step.Locator),
			Nth:      step.Nth,
		})
		if err != nil {
			return paths, err
		}

		path := flowScreenshotPath(basePath, target.Side, multi)
		if err := writeFlowScreenshotFile(path, data); err != nil {
			return paths, err
		}
		paths[target.Side] = path
	}

	return paths, nil
}

func executeFlowActionOnSides(ctx context.Context, client *rpc.Client, state flowExecutionState, step flowStep, action api.Action) error {
	for _, sessionID := range flowStepSessions(state, step) {
		res, err := client.ActSession(ctx, api.ActSessionRequest{
			SessionID: sessionID,
			Action:    action,
		})
		if err != nil {
			return err
		}
		if !res.Result.OK {
			if strings.TrimSpace(res.Result.Message) != "" {
				return errors.New(strings.TrimSpace(res.Result.Message))
			}
			return fmt.Errorf("%s failed", action.Kind)
		}
	}
	return nil
}

type flowSessionTarget struct {
	Side      string
	SessionID string
}

func flowStepTargets(state flowExecutionState, step flowStep) []flowSessionTarget {
	switch flowStepSide(step) {
	case "old":
		return []flowSessionTarget{{Side: "old", SessionID: state.Old.SessionID}}
	case "new":
		return []flowSessionTarget{{Side: "new", SessionID: state.New.SessionID}}
	default:
		return []flowSessionTarget{
			{Side: "old", SessionID: state.Old.SessionID},
			{Side: "new", SessionID: state.New.SessionID},
		}
	}
}

func flowStepSessions(state flowExecutionState, step flowStep) []string {
	targets := flowStepTargets(state, step)
	sessionIDs := make([]string, 0, len(targets))
	for _, target := range targets {
		sessionIDs = append(sessionIDs, target.SessionID)
	}
	return sessionIDs
}

func flowStepSide(step flowStep) string {
	side := strings.TrimSpace(step.Side)
	if side == "" {
		return "both"
	}
	return side
}

func flowStepName(step flowStep) string {
	if strings.TrimSpace(step.Name) != "" {
		return strings.TrimSpace(step.Name)
	}
	if strings.TrimSpace(step.Action) != "" {
		return strings.TrimSpace(step.Action)
	}
	return "step"
}

func resolveFlowLocator(ctx context.Context, client *rpc.Client, sessionID string, locator string, selection nodeSelectionOptions) (api.Node, error) {
	observation, err := observeTreeForFind(ctx, client, sessionID)
	if err != nil {
		return api.Node{}, err
	}
	terms, err := parseFlowLocator(locator)
	if err != nil {
		return api.Node{}, err
	}
	matches := selectNodes(observation.Tree, func(node api.Node) bool {
		for _, term := range terms {
			if !matchesFlowLocatorTerm(node, term) {
				return false
			}
		}
		return true
	})
	return chooseNode(matches, locator, selection)
}

func parseFlowLocator(locator string) ([]flowSelectorTerm, error) {
	trimmed := strings.TrimSpace(locator)
	if trimmed == "" {
		return nil, errors.New("locator must not be empty")
	}
	if strings.HasPrefix(trimmed, "@e") {
		return []flowSelectorTerm{{Kind: "ref", Value: trimmed}}, nil
	}
	parts := strings.Split(trimmed, "&")
	terms := make([]flowSelectorTerm, 0, len(parts))
	for _, part := range parts {
		kind, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return nil, fmt.Errorf("invalid locator %q", locator)
		}
		kind = strings.TrimSpace(kind)
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("invalid locator %q: value must not be empty", locator)
		}
		switch kind {
		case "role", "name", "text", "label", "testid", "href":
			terms = append(terms, flowSelectorTerm{Kind: kind, Value: value})
		default:
			return nil, fmt.Errorf("invalid locator %q: supported kinds are role, name, text, label, testid, href, or @eN", locator)
		}
	}
	return terms, nil
}

func matchesFlowLocatorTerm(node api.Node, term flowSelectorTerm) bool {
	switch term.Kind {
	case "ref":
		return strings.TrimSpace(node.Ref) == term.Value
	case "role":
		return normalizeFindValue(node.Role) == normalizeFindValue(term.Value)
	case "name":
		return nodeMatches(node, term.Value)
	case "text":
		return compareSelectorContains(node.Text, term.Value)
	case "label":
		if !node.Editable && !node.Selectable && !strings.EqualFold(node.Role, "textbox") && !strings.EqualFold(node.Role, "combobox") {
			return false
		}
		return nodeMatches(node, term.Value)
	case "testid":
		return compareSelectorContains(firstNonEmpty(node.Attrs["data-testid"], node.Attrs["data-test"]), term.Value)
	case "href":
		return compareSelectorContains(node.Attrs["href"], term.Value)
	default:
		return false
	}
}

func compareSelectorContains(value string, needle string) bool {
	return strings.Contains(normalizeFindValue(value), normalizeFindValue(needle))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func summarizeFlowReport(scenarios []flowScenarioReport) flowReportSummary {
	summary := flowReportSummary{
		TotalScenarios: len(scenarios),
	}
	for _, scenario := range scenarios {
		if scenario.Status == "failed" {
			summary.FailedScenarios++
		} else {
			summary.CompletedScenarios++
		}
		for _, step := range scenario.Steps {
			summary.TotalSteps++
			if step.Status == "failed" {
				summary.FailedSteps++
			}
			if len(step.Compare) == 0 {
				continue
			}
			summary.TotalCompares++
			var compareReport flowCompareReport
			if err := json.Unmarshal(step.Compare, &compareReport); err == nil && !compareReport.Summary.Same {
				summary.DifferentCompares++
			}
		}
	}
	return summary
}

func printFlowReport(w io.Writer, report flowReport) {
	fmt.Fprintf(w, "manifest: %s\n", report.Manifest)
	fmt.Fprintf(w, "scenarios: %d completed=%d failed=%d\n", report.Summary.TotalScenarios, report.Summary.CompletedScenarios, report.Summary.FailedScenarios)
	fmt.Fprintf(w, "steps: %d failed=%d\n", report.Summary.TotalSteps, report.Summary.FailedSteps)
	fmt.Fprintf(w, "compares: %d different=%d\n", report.Summary.TotalCompares, report.Summary.DifferentCompares)
	for _, scenario := range report.Scenarios {
		fmt.Fprintf(w, "\n[%s] %s", scenario.Status, scenario.Name)
		if scenario.Matrix != "" {
			fmt.Fprintf(w, " matrix=%s", scenario.Matrix)
		}
		fmt.Fprintln(w)
		if scenario.Error != "" {
			fmt.Fprintf(w, "error: %s\n", scenario.Error)
		}
		for _, step := range scenario.Steps {
			fmt.Fprintf(w, "- %s %s (%s)\n", step.Action, step.Name, step.Status)
			if step.Error != "" {
				fmt.Fprintf(w, "  error: %s\n", step.Error)
			}
			if len(step.Screenshots) > 0 {
				sides := make([]string, 0, len(step.Screenshots))
				for side := range step.Screenshots {
					sides = append(sides, side)
				}
				sort.Strings(sides)
				for _, side := range sides {
					fmt.Fprintf(w, "  screenshot[%s]: %s\n", side, step.Screenshots[side])
				}
			}
		}
	}
}

func writeFlowJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func flowScreenshotPath(path string, side string, multi bool) string {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	ext := filepath.Ext(cleaned)
	if ext == "" {
		cleaned += ".png"
		ext = ".png"
	}
	if !multi {
		return cleaned
	}
	base := strings.TrimSuffix(cleaned, ext)
	return base + "-" + side + ext
}

func writeFlowScreenshotFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

func printFlowHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl flow run --manifest <file> [--scenario <name>] [--matrix <name>] [--continue-on-error] [--output-json <file>] [--json]")
	fmt.Fprintln(w, "")
	printDocLink(w, "flow guide", aiFlowDocURL)
	printDocLink(w, "migration playbook", migrationPlaybookDocURL)
	printDocLink(w, "ai guide", aiUsageDocURL)
}

func printFlowRunHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: nxctl flow run --manifest <file> [--scenario <name>] [--matrix <name>] [--continue-on-error] [--output-json <file>] [--json]")
	fmt.Fprintln(w, "")
	printDocLink(w, "flow guide", aiFlowDocURL)
	printDocLink(w, "migration playbook", migrationPlaybookDocURL)
	printDocLink(w, "ai guide", aiUsageDocURL)
}

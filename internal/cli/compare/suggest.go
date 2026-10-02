package comparecmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	defaultIdentityModel    = "jev-1.13.0"
	identityQuestionVersion = "nexus-element-identity-v1"
	identityMaxCompareBytes = 64 << 20
	identityFeatureLength   = 240
	identityNoMatch         = "none"
	identityUnknown         = "unknown"
)

type identitySuggestionArguments struct {
	CompareJSON    string
	Output         string
	OutputJSON     string
	Model          string
	MinProbability float64
	MinMargin      float64
	Limit          int
	MaxCandidates  int
	Timeout        int
	Promote        bool
	DryRun         bool
	JSON           bool
}

// Only these bounded semantic fields are sent to the remote judge. In particular,
// form values, full URLs, fingerprints, screenshots, and CSS stay in the report.
type identityFeatures struct {
	Role        string `json:"role,omitempty"`
	Name        string `json:"name,omitempty"`
	Label       string `json:"label,omitempty"`
	Text        string `json:"text,omitempty"`
	HrefPath    string `json:"href_path,omitempty"`
	TestID      string `json:"testid,omitempty"`
	Type        string `json:"type,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

type identityContext struct {
	Node      identityFeatures   `json:"node"`
	Ancestors []identityFeatures `json:"ancestors,omitempty"`
	Previous  []identityFeatures `json:"previous,omitempty"`
	Next      []identityFeatures `json:"next,omitempty"`
}

type identityCandidate struct {
	Key         string `json:"key"`
	Ref         string `json:"ref"`
	Fingerprint string `json:"fingerprint"`
	Score       int    `json:"retrieval_score"`
	index       int
}

type identityTask struct {
	Old        compareSnapshotNode
	Candidates []identityCandidate
	State      identityState
}

type identityState struct {
	Old        identityContext            `json:"old"`
	Candidates map[string]identityContext `json:"candidates"`
}

type identityJudgment struct {
	Old           string               `json:"old"`
	Candidates    []identityCandidate  `json:"candidates"`
	RequestSHA256 string               `json:"request_sha256,omitempty"`
	Response      *jevIdentityResponse `json:"response,omitempty"`
	Probability   float64              `json:"probability"`
	SameElement   float64              `json:"same_element_probability"`
	Margin        float64              `json:"margin"`
	Decision      compareDecision      `json:"decision"`
}

type identitySuggestionSummary struct {
	UnmatchedOld int `json:"unmatched_old"`
	UnmatchedNew int `json:"unmatched_new"`
	Considered   int `json:"considered"`
	Requests     int `json:"requests"`
	Skipped      int `json:"skipped"`
	High         int `json:"high"`
	Tentative    int `json:"tentative"`
	Unknown      int `json:"unknown"`
}

type identitySuggestionReport struct {
	SchemaVersion   int                       `json:"schema_version"`
	Provider        string                    `json:"provider"`
	RequestedModel  string                    `json:"requested_model"`
	QuestionVersion string                    `json:"question_version"`
	CompareSHA256   string                    `json:"compare_sha256"`
	MinProbability  float64                   `json:"min_probability"`
	MinMargin       float64                   `json:"min_margin"`
	Promote         bool                      `json:"promote"`
	DryRun          bool                      `json:"dry_run"`
	Output          string                    `json:"output,omitempty"`
	OutputJSON      string                    `json:"output_json,omitempty"`
	Summary         identitySuggestionSummary `json:"summary"`
	Judgments       []identityJudgment        `json:"judgments"`
	Requests        []jevIdentityRequest      `json:"requests,omitempty"`
}

type identityEvaluator interface {
	evaluate(context.Context, jevIdentityRequest) (jevIdentityResponse, error)
}

func loadIdentityCompare(path string) (compareReport, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return compareReport{}, "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, identityMaxCompareBytes+1))
	if err != nil {
		return compareReport{}, "", err
	}
	if len(data) > identityMaxCompareBytes {
		return compareReport{}, "", errors.New("compare JSON exceeds the 64 MiB suggestion limit")
	}
	var report compareReport
	if err := json.Unmarshal(data, &report); err != nil {
		return compareReport{}, "", errors.New("invalid compare JSON")
	}
	if report.MatchingDebug == nil {
		return compareReport{}, "", errors.New("suggest-decisions requires a single-page compare JSON with --matching-debug")
	}
	return report, fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

func identityUnmatchedIndices(nodes []compareSnapshotNode, debug []compareMatchingDebugNode, matches []compareMatchingDebugMatch, old bool) ([]int, error) {
	byRef := make(map[string]int, len(nodes))
	for index, node := range nodes {
		if node.Ref == "" {
			continue
		}
		if _, exists := byRef[node.Ref]; exists {
			return nil, errors.New("compare snapshot contains duplicate node refs")
		}
		byRef[node.Ref] = index
	}
	matched := map[string]bool{}
	for _, match := range matches {
		node := match.New
		if old {
			node = match.Old
		}
		matched[node.Ref] = true
	}
	seen := map[string]bool{}
	indices := make([]int, 0, len(debug))
	for _, node := range debug {
		if node.Ref == "" {
			continue
		}
		index, exists := byRef[node.Ref]
		if !exists || node.Fingerprint == "" || nodes[index].Fingerprint != node.Fingerprint {
			return nil, errors.New("unmatched debug node does not resolve to its snapshot ref and fingerprint")
		}
		if seen[node.Ref] || matched[node.Ref] {
			return nil, errors.New("compare matching debug reuses an unmatched node")
		}
		seen[node.Ref] = true
		indices = append(indices, index)
	}
	compareSortNodeIndicesBySequence(nodes, indices)
	return indices, nil
}

func buildIdentityTasks(report compareReport, args identitySuggestionArguments) ([]identityTask, error) {
	debug := report.MatchingDebug
	oldIndices, err := identityUnmatchedIndices(report.Old.Nodes, debug.UnmatchedOld, debug.Matches, true)
	if err != nil {
		return nil, err
	}
	newIndices, err := identityUnmatchedIndices(report.New.Nodes, debug.UnmatchedNew, debug.Matches, false)
	if err != nil {
		return nil, err
	}
	tasks := make([]identityTask, 0, min(args.Limit, len(oldIndices)))
	for _, oldIndex := range oldIndices {
		if len(tasks) == args.Limit {
			break
		}
		oldNode := report.Old.Nodes[oldIndex]
		candidates := make([]identityCandidate, 0, len(newIndices))
		for _, newIndex := range newIndices {
			newNode := report.New.Nodes[newIndex]
			score := identityRetrievalScore(oldNode, newNode)
			if score == 0 {
				continue
			}
			candidates = append(candidates, identityCandidate{Ref: newNode.Ref, Fingerprint: newNode.Fingerprint, Score: score, index: newIndex})
		}
		slices.SortStableFunc(candidates, func(a, b identityCandidate) int { return b.Score - a.Score })
		candidates = candidates[:min(args.MaxCandidates, len(candidates))]
		state := identityState{Old: buildIdentityContext(report.Old.Nodes, oldIndex), Candidates: map[string]identityContext{}}
		for index := range candidates {
			candidates[index].Key = fmt.Sprintf("candidate_%d", index+1)
			state.Candidates[candidates[index].Key] = buildIdentityContext(report.New.Nodes, candidates[index].index)
		}
		tasks = append(tasks, identityTask{Old: oldNode, Candidates: candidates, State: state})
	}
	return tasks, nil
}

func identityRetrievalScore(old, next compareSnapshotNode) int {
	old.MatchBounds, next.MatchBounds = old.Bounds, next.Bounds
	score := compareHeuristicNodeScore(old, next).Score
	// Retrieval may cross roles when semantic evidence survives an a11y or markup
	// change. This score only selects candidates; it never accepts a match.
	if old.TestID != "" && old.TestID == next.TestID {
		score += 100
	}
	if old.Name != "" && old.Name == next.Name {
		score += 40
	}
	if old.Label != "" && old.Label == next.Label {
		score += 40
	}
	if path := identityHrefPath(old.Href); path != "" && path == identityHrefPath(next.Href) {
		score += 60
	}
	return score
}

func identityHrefPath(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Opaque != "" || (parsed.Scheme != "" && parsed.Scheme != "https" && parsed.Scheme != "http") {
		return ""
	}
	return parsed.EscapedPath()
}

func identityFeature(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > identityFeatureLength {
		return string(runes[:identityFeatureLength]) + "..."
	}
	return string(runes)
}

func buildIdentityFeatures(node compareSnapshotNode) identityFeatures {
	return identityFeatures{
		Role: identityFeature(node.Role), Name: identityFeature(node.Name), Label: identityFeature(node.Label),
		Text: identityFeature(node.Text), HrefPath: identityFeature(identityHrefPath(node.Href)), TestID: identityFeature(node.TestID),
		Type: identityFeature(node.TypeAttr), Placeholder: identityFeature(node.Placeholder),
	}
}

func buildIdentityContext(nodes []compareSnapshotNode, index int) identityContext {
	result := identityContext{Node: buildIdentityFeatures(nodes[index])}
	ancestors := make([]int, 0)
	path := nodes[index].StructurePath
	for other, node := range nodes {
		if node.StructurePath != "" && strings.HasPrefix(path, node.StructurePath+">") {
			ancestors = append(ancestors, other)
		}
	}
	slices.SortFunc(ancestors, func(a, b int) int { return len(nodes[b].StructurePath) - len(nodes[a].StructurePath) })
	for _, ancestor := range ancestors[:min(3, len(ancestors))] {
		result.Ancestors = append(result.Ancestors, buildIdentityFeatures(nodes[ancestor]))
	}
	order := compareAllNodeIndices(nodes)
	compareSortNodeIndicesBySequence(nodes, order)
	position := slices.Index(order, index)
	for _, previous := range order[max(0, position-2):position] {
		result.Previous = append(result.Previous, buildIdentityFeatures(nodes[previous]))
	}
	for _, next := range order[position+1 : min(len(order), position+3)] {
		result.Next = append(result.Next, buildIdentityFeatures(nodes[next]))
	}
	return result
}

func judgeIdentityTasks(ctx context.Context, tasks []identityTask, args identitySuggestionArguments, evaluator identityEvaluator, report *identitySuggestionReport) error {
	for _, task := range tasks {
		if err := ctx.Err(); err != nil {
			return err
		}
		judgment := identityJudgment{
			Old: task.Old.Ref, Candidates: task.Candidates,
			Decision: compareDecision{SchemaVersion: 1, Kind: "pair", Old: task.Old.Ref, OldFingerprint: task.Old.Fingerprint, New: "?", Confidence: "unknown", Reason: "no eligible candidate"},
		}
		if len(task.Candidates) > 0 {
			request := buildJevIdentityRequest(task, args.Model)
			data, err := json.Marshal(request)
			if err != nil {
				return err
			}
			if len(data) > jevIdentityMaxRequestBytes {
				return errors.New("Jev request exceeds the 32 KiB context limit; reduce --max-candidates or narrow the compare scope")
			}
			judgment.RequestSHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
			report.Summary.Requests++
			if args.DryRun {
				report.Requests = append(report.Requests, request)
				judgment.Decision.Reason = "dry-run; not evaluated"
			} else {
				response, err := evaluator.evaluate(ctx, request)
				if err != nil {
					return fmt.Errorf("identity request %d failed: %w", report.Summary.Requests, err)
				}
				if err := validateJevIdentityResponse(request, response); err != nil {
					return err
				}
				judgment.Response = &response
				applyIdentityAnswer(task, args, &judgment)
			}
		}
		report.Judgments = append(report.Judgments, judgment)
	}
	// Independent questions can select the same new occurrence. Keep every
	// conflicting proposal unresolved, including conflicts with tentative pairs.
	owners := map[string]int{}
	for _, judgment := range report.Judgments {
		if judgment.Decision.New != "?" {
			owners[judgment.Decision.New]++
		}
	}
	for index := range report.Judgments {
		decision := &report.Judgments[index].Decision
		if owners[decision.New] > 1 {
			decision.New, decision.NewFingerprint, decision.Confidence = "?", "", "unknown"
			decision.Reason = "multiple old nodes selected the same new node; review required"
		}
		switch decision.Confidence {
		case "high":
			report.Summary.High++
		case "tentative":
			report.Summary.Tentative++
		default:
			report.Summary.Unknown++
		}
	}
	return nil
}

func applyIdentityAnswer(task identityTask, args identitySuggestionArguments, judgment *identityJudgment) {
	answer := judgment.Response.Answers["correspondence"]
	judgment.Probability = *answer.Probabilities[answer.Choice]
	second := 0.0
	for key, probability := range answer.Probabilities {
		if key != answer.Choice {
			second = max(second, *probability)
		}
	}
	judgment.Margin = judgment.Probability - second
	judgment.Decision.DecidedBy = "jev:" + judgment.Response.Model
	judgment.Decision.Context = identityQuestionVersion
	judgment.Decision.Reason = "model selected " + answer.Choice + "; review required"
	for _, candidate := range task.Candidates {
		if candidate.Key != answer.Choice {
			continue
		}
		judgment.SameElement = *judgment.Response.Answers["same_"+candidate.Key].Noul
		decision := &judgment.Decision
		decision.New, decision.NewFingerprint, decision.Confidence = candidate.Ref, candidate.Fingerprint, "tentative"
		eligible := judgment.Probability >= args.MinProbability && judgment.SameElement >= args.MinProbability && judgment.Margin >= args.MinMargin
		if eligible && args.Promote {
			decision.Confidence = "high"
		}
		decision.Reason = fmt.Sprintf("Jev candidate probability %.4f, same-element probability %.4f, margin %.4f; %s", judgment.Probability, judgment.SameElement, judgment.Margin, decision.Confidence)
	}
}

func runIdentitySuggestions(ctx context.Context, args identitySuggestionArguments, stdout, stderr io.Writer, evaluator identityEvaluator) int {
	fail := func(err error) int { fmt.Fprintln(stderr, err); return 1 }
	if err := validateIdentitySuggestionArguments(args); err != nil {
		return fail(err)
	}
	if !args.DryRun && args.OutputJSON == "" {
		args.OutputJSON = args.Output + ".review.json"
	}
	paths := []string{args.OutputJSON}
	if !args.DryRun {
		paths = append(paths, args.Output)
	}
	if err := preflightIdentityOutputPaths(paths); err != nil {
		return fail(err)
	}
	comparison, hash, err := loadIdentityCompare(args.CompareJSON)
	if err != nil {
		return fail(err)
	}
	tasks, err := buildIdentityTasks(comparison, args)
	if err != nil {
		return fail(err)
	}
	if !args.DryRun && evaluator == nil && slices.ContainsFunc(tasks, func(task identityTask) bool { return len(task.Candidates) > 0 }) {
		evaluator, err = newJevIdentityClient(os.Getenv("TYPESAFE_API_KEY"))
		if err != nil {
			return fail(err)
		}
	}
	report := identitySuggestionReport{
		SchemaVersion: 1, Provider: "jev", RequestedModel: args.Model, QuestionVersion: identityQuestionVersion, CompareSHA256: hash,
		MinProbability: args.MinProbability, MinMargin: args.MinMargin, Promote: args.Promote, DryRun: args.DryRun,
		OutputJSON: args.OutputJSON, Judgments: make([]identityJudgment, 0, len(tasks)),
		Summary: identitySuggestionSummary{UnmatchedOld: len(comparison.MatchingDebug.UnmatchedOld), UnmatchedNew: len(comparison.MatchingDebug.UnmatchedNew), Considered: len(tasks), Skipped: len(comparison.MatchingDebug.UnmatchedOld) - len(tasks)},
	}
	if !args.DryRun {
		report.Output = args.Output
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(args.Timeout)*time.Millisecond)
	defer cancel()
	if err := judgeIdentityTasks(ctx, tasks, args, evaluator, &report); err != nil {
		return fail(err)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	outputs := []identityOutputFile{}
	if !args.DryRun {
		decisions := make([]compareDecision, 0, len(report.Judgments))
		for _, judgment := range report.Judgments {
			decisions = append(decisions, judgment.Decision)
		}
		if validation := validateCompareDecisions(decisions, &comparison); validation.Summary.Errors > 0 {
			return fail(errors.New("suggested decisions failed compare validation"))
		}
		var data bytes.Buffer
		if err := printCompareDecisionJSONL(&data, decisions); err != nil {
			return fail(err)
		}
		outputs = append(outputs, identityOutputFile{Path: args.Output, Data: data.Bytes()})
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fail(err)
	}
	data = append(data, '\n')
	if args.OutputJSON != "" {
		outputs = append(outputs, identityOutputFile{Path: args.OutputJSON, Data: data})
	}
	if err := writeIdentityOutputFiles(outputs); err != nil {
		return fail(err)
	}
	if args.JSON || args.DryRun {
		if _, err := stdout.Write(data); err != nil {
			return fail(err)
		}
	} else {
		fmt.Fprintf(stdout, "identity suggestions: %d high, %d tentative, %d unknown; %d skipped\n", report.Summary.High, report.Summary.Tentative, report.Summary.Unknown, report.Summary.Skipped)
		fmt.Fprintf(stdout, "decisions: %s\nreview: %s\n", report.Output, report.OutputJSON)
	}
	return 0
}

type identityOutputFile struct {
	Path string
	Data []byte
}

func preflightIdentityOutputPaths(paths []string) error {
	seen := map[string]bool{}
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			if err != nil {
				return err
			}
			return fmt.Errorf("suggestion output already exists: %s", path)
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(path))
		if err != nil {
			return err
		}
		absolute, err := filepath.Abs(filepath.Join(parent, filepath.Base(path)))
		if err != nil {
			return err
		}
		if seen[absolute] {
			return errors.New("decision and review outputs must use different paths")
		}
		seen[absolute] = true
	}
	return nil
}

func writeIdentityOutputFiles(outputs []identityOutputFile) (err error) {
	created := make([]string, 0, len(outputs))
	defer func() {
		if err != nil {
			for _, path := range created {
				os.Remove(path)
			}
		}
	}()
	for _, output := range outputs {
		file, openErr := os.OpenFile(output.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if openErr != nil {
			return openErr
		}
		created = append(created, output.Path)
		_, writeErr := file.Write(output.Data)
		closeErr := file.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			return err
		}
	}
	return nil
}

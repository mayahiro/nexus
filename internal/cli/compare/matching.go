package comparecmd

import (
	"errors"
	"slices"
	"strings"
)

const (
	compareMatchModeExact     = defaultCompareMatchMode
	compareMatchModeStable    = "stable"
	compareMatchModeHeuristic = "heuristic"

	compareHeuristicMinimumScore  = 75
	compareHeuristicMinimumMargin = 10
)

const (
	compareStablePriorityTestID = iota
	compareStablePriorityID
	compareStablePriorityHref
	compareStablePriorityLabel
	compareStablePriorityRoleName
	compareStablePriorityAttr
	compareStablePriorityPlaceholder
	compareStablePriorityFingerprint
)

type compareNodeMatch struct {
	OldIndex  int
	NewIndex  int
	MatchedBy string
	Score     int
	Reasons   []string
}

type compareNodeMatchResult struct {
	Matches      []compareNodeMatch
	UnmatchedOld []int
	UnmatchedNew []int
}

type compareStableIdentityKey struct {
	Priority int
	Kind     string
	Value    string
}

type compareHeuristicScore struct {
	Score    int
	Reasons  []string
	Semantic bool
	Strong   bool
}

type compareBestCandidate struct {
	Index   int
	Score   int
	Second  int
	Reasons []string
}

func normalizeCompareMatchMode(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return defaultCompareMatchMode, nil
	}
	switch normalized {
	case compareMatchModeExact, compareMatchModeStable, compareMatchModeHeuristic:
		return normalized, nil
	default:
		return "", errors.New("match-mode must be exact, stable, or heuristic")
	}
}

func compareMatchNodes(oldNodes []compareSnapshotNode, newNodes []compareSnapshotNode, mode string) compareNodeMatchResult {
	normalized, err := normalizeCompareMatchMode(mode)
	if err != nil {
		normalized = defaultCompareMatchMode
	}

	oldIndices := compareAllNodeIndices(oldNodes)
	newIndices := compareAllNodeIndices(newNodes)
	switch normalized {
	case compareMatchModeStable:
		return compareStableNodeMatches(oldNodes, newNodes)
	case compareMatchModeHeuristic:
		stable := compareStableNodeMatches(oldNodes, newNodes)
		return compareHeuristicNodeMatches(oldNodes, newNodes, stable)
	default:
		return compareExactNodeMatches(oldNodes, newNodes, oldIndices, newIndices, "", nil)
	}
}

func compareStableNodeMatches(oldNodes []compareSnapshotNode, newNodes []compareSnapshotNode) compareNodeMatchResult {
	unmatchedOld := compareNodeIndexSet(compareAllNodeIndices(oldNodes))
	unmatchedNew := compareNodeIndexSet(compareAllNodeIndices(newNodes))
	matches := make([]compareNodeMatch, 0)

	for priority := compareStablePriorityTestID; priority <= compareStablePriorityFingerprint; priority++ {
		oldByKey := compareStableKeyIndex(oldNodes, unmatchedOld, priority)
		newByKey := compareStableKeyIndex(newNodes, unmatchedNew, priority)
		keys := compareSharedStableKeys(oldByKey, newByKey)
		for _, key := range keys {
			oldIndexes := oldByKey[key]
			newIndexes := newByKey[key]
			if len(oldIndexes) != 1 || len(newIndexes) != 1 {
				continue
			}
			oldIndex := oldIndexes[0]
			newIndex := newIndexes[0]
			if _, ok := unmatchedOld[oldIndex]; !ok {
				continue
			}
			if _, ok := unmatchedNew[newIndex]; !ok {
				continue
			}
			delete(unmatchedOld, oldIndex)
			delete(unmatchedNew, newIndex)
			matches = append(matches, compareNodeMatch{
				OldIndex:  oldIndex,
				NewIndex:  newIndex,
				MatchedBy: "stable:" + key.Kind,
				Reasons:   []string{key.Kind},
			})
		}
	}

	exact := compareExactNodeMatches(
		oldNodes,
		newNodes,
		compareSortedNodeIndexSet(unmatchedOld),
		compareSortedNodeIndexSet(unmatchedNew),
		"fingerprint",
		[]string{"fingerprint"},
	)
	matches = append(matches, exact.Matches...)
	return compareNodeMatchResult{
		Matches:      matches,
		UnmatchedOld: exact.UnmatchedOld,
		UnmatchedNew: exact.UnmatchedNew,
	}
}

func compareHeuristicNodeMatches(oldNodes []compareSnapshotNode, newNodes []compareSnapshotNode, base compareNodeMatchResult) compareNodeMatchResult {
	matches := append([]compareNodeMatch(nil), base.Matches...)
	heuristic := compareHeuristicUnmatchedNodes(oldNodes, newNodes, base.UnmatchedOld, base.UnmatchedNew)
	matches = append(matches, heuristic.Matches...)
	return compareNodeMatchResult{
		Matches:      matches,
		UnmatchedOld: heuristic.UnmatchedOld,
		UnmatchedNew: heuristic.UnmatchedNew,
	}
}

func compareExactNodeMatches(oldNodes []compareSnapshotNode, newNodes []compareSnapshotNode, oldIndices []int, newIndices []int, matchedBy string, reasons []string) compareNodeMatchResult {
	oldGroups := compareNodeGroupsByFingerprint(oldNodes, oldIndices)
	newGroups := compareNodeGroupsByFingerprint(newNodes, newIndices)
	keys := compareNodeGroupKeys(oldGroups, newGroups)
	matches := make([]compareNodeMatch, 0)
	unmatchedOld := make([]int, 0)
	unmatchedNew := make([]int, 0)

	for _, key := range keys {
		oldGroup := oldGroups[key]
		newGroup := newGroups[key]
		maxLen := max(len(oldGroup), len(newGroup))
		for i := 0; i < maxLen; i++ {
			switch {
			case i >= len(oldGroup):
				unmatchedNew = append(unmatchedNew, newGroup[i])
			case i >= len(newGroup):
				unmatchedOld = append(unmatchedOld, oldGroup[i])
			default:
				matches = append(matches, compareNodeMatch{
					OldIndex:  oldGroup[i],
					NewIndex:  newGroup[i],
					MatchedBy: matchedBy,
					Reasons:   append([]string(nil), reasons...),
				})
			}
		}
	}

	slices.Sort(unmatchedOld)
	slices.Sort(unmatchedNew)
	return compareNodeMatchResult{
		Matches:      matches,
		UnmatchedOld: unmatchedOld,
		UnmatchedNew: unmatchedNew,
	}
}

func compareNodeGroupsByFingerprint(nodes []compareSnapshotNode, indices []int) map[string][]int {
	grouped := make(map[string][]int, len(indices))
	for _, index := range indices {
		node := nodes[index]
		grouped[node.Fingerprint] = append(grouped[node.Fingerprint], index)
	}
	for key := range grouped {
		slices.SortFunc(grouped[key], func(a int, b int) int {
			aKey := compareNodeSortKey(nodes[a])
			bKey := compareNodeSortKey(nodes[b])
			switch {
			case aKey < bKey:
				return -1
			case aKey > bKey:
				return 1
			default:
				return 0
			}
		})
	}
	return grouped
}

func compareNodeGroupKeys(oldGroups map[string][]int, newGroups map[string][]int) []string {
	keys := make([]string, 0, len(oldGroups)+len(newGroups))
	seen := map[string]struct{}{}
	for key := range oldGroups {
		keys = append(keys, key)
		seen[key] = struct{}{}
	}
	for key := range newGroups {
		if _, ok := seen[key]; ok {
			continue
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func compareStableKeyIndex(nodes []compareSnapshotNode, unmatched map[int]struct{}, priority int) map[compareStableIdentityKey][]int {
	byKey := map[compareStableIdentityKey][]int{}
	for index := range unmatched {
		for _, key := range compareStableIdentityKeys(nodes[index]) {
			if key.Priority != priority {
				continue
			}
			byKey[key] = append(byKey[key], index)
		}
	}
	return byKey
}

func compareSharedStableKeys(oldByKey map[compareStableIdentityKey][]int, newByKey map[compareStableIdentityKey][]int) []compareStableIdentityKey {
	keys := make([]compareStableIdentityKey, 0)
	for key := range oldByKey {
		if _, ok := newByKey[key]; ok {
			keys = append(keys, key)
		}
	}
	slices.SortFunc(keys, func(a compareStableIdentityKey, b compareStableIdentityKey) int {
		switch {
		case a.Priority < b.Priority:
			return -1
		case a.Priority > b.Priority:
			return 1
		case a.Kind < b.Kind:
			return -1
		case a.Kind > b.Kind:
			return 1
		case a.Value < b.Value:
			return -1
		case a.Value > b.Value:
			return 1
		default:
			return 0
		}
	})
	return keys
}

func compareStableIdentityKeys(node compareSnapshotNode) []compareStableIdentityKey {
	keys := make([]compareStableIdentityKey, 0, 8)
	role := strings.ToLower(strings.TrimSpace(node.Role))
	tag := strings.ToLower(strings.TrimSpace(node.Tag))
	appendKey := func(priority int, kind string, parts ...string) {
		values := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				return
			}
			values = append(values, trimmed)
		}
		keys = append(keys, compareStableIdentityKey{
			Priority: priority,
			Kind:     kind,
			Value:    strings.Join(values, "|"),
		})
	}

	appendKey(compareStablePriorityTestID, "testid", node.TestID)
	appendKey(compareStablePriorityID, "id", node.IDAttr)
	appendKey(compareStablePriorityHref, "href", node.Href)
	if compareStableSupportsLabel(node) {
		appendKey(compareStablePriorityLabel, "label", role, node.Name)
	}
	appendKey(compareStablePriorityRoleName, "role-name", role, node.Name)
	if tag != "" && (strings.TrimSpace(node.NameAttr) != "" || strings.TrimSpace(node.TypeAttr) != "") {
		keys = append(keys, compareStableIdentityKey{
			Priority: compareStablePriorityAttr,
			Kind:     "attr",
			Value:    strings.Join([]string{tag, strings.TrimSpace(node.NameAttr), strings.TrimSpace(node.TypeAttr)}, "|"),
		})
	}
	appendKey(compareStablePriorityPlaceholder, "placeholder", role, node.Placeholder)
	appendKey(compareStablePriorityFingerprint, "fingerprint", node.Fingerprint)
	return keys
}

func compareStableSupportsLabel(node compareSnapshotNode) bool {
	if compareSupportsLabelLocator(node) {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(node.Role)) {
	case "checkbox", "radio", "searchbox", "spinbutton", "slider":
		return true
	default:
		return false
	}
}

func compareHeuristicUnmatchedNodes(oldNodes []compareSnapshotNode, newNodes []compareSnapshotNode, oldIndices []int, newIndices []int) compareNodeMatchResult {
	scores := map[[2]int]compareHeuristicScore{}
	oldBest := map[int]compareBestCandidate{}
	newBest := map[int]compareBestCandidate{}

	for _, oldIndex := range oldIndices {
		for _, newIndex := range newIndices {
			score := compareHeuristicNodeScore(oldNodes[oldIndex], newNodes[newIndex])
			if score.Score < compareHeuristicMinimumScore || !score.Semantic {
				continue
			}
			pair := [2]int{oldIndex, newIndex}
			scores[pair] = score
			oldBest[oldIndex] = compareUpdateBestCandidate(oldBest[oldIndex], newIndex, score)
			newBest[newIndex] = compareUpdateBestCandidate(newBest[newIndex], oldIndex, score)
		}
	}

	matchedOld := map[int]struct{}{}
	matchedNew := map[int]struct{}{}
	matches := make([]compareNodeMatch, 0)
	for _, oldIndex := range oldIndices {
		best, ok := oldBest[oldIndex]
		if !ok || !compareBestCandidateMarginOK(best) {
			continue
		}
		newIndex := best.Index
		reverse, ok := newBest[newIndex]
		if !ok || reverse.Index != oldIndex || !compareBestCandidateMarginOK(reverse) {
			continue
		}
		if _, ok := matchedOld[oldIndex]; ok {
			continue
		}
		if _, ok := matchedNew[newIndex]; ok {
			continue
		}
		score := scores[[2]int{oldIndex, newIndex}]
		matchedOld[oldIndex] = struct{}{}
		matchedNew[newIndex] = struct{}{}
		matches = append(matches, compareNodeMatch{
			OldIndex:  oldIndex,
			NewIndex:  newIndex,
			MatchedBy: "heuristic",
			Score:     score.Score,
			Reasons:   score.Reasons,
		})
	}

	return compareNodeMatchResult{
		Matches:      matches,
		UnmatchedOld: compareUnmatchedNodeIndices(oldIndices, matchedOld),
		UnmatchedNew: compareUnmatchedNodeIndices(newIndices, matchedNew),
	}
}

func compareUpdateBestCandidate(current compareBestCandidate, index int, score compareHeuristicScore) compareBestCandidate {
	if current.Score == 0 {
		return compareBestCandidate{Index: index, Score: score.Score, Second: -1, Reasons: score.Reasons}
	}
	if score.Score > current.Score {
		return compareBestCandidate{Index: index, Score: score.Score, Second: current.Score, Reasons: score.Reasons}
	}
	if score.Score > current.Second {
		current.Second = score.Score
	}
	return current
}

func compareBestCandidateMarginOK(candidate compareBestCandidate) bool {
	if candidate.Score < compareHeuristicMinimumScore {
		return false
	}
	if candidate.Second < 0 {
		return true
	}
	return candidate.Score-candidate.Second >= compareHeuristicMinimumMargin
}

func compareHeuristicNodeScore(oldNode compareSnapshotNode, newNode compareSnapshotNode) compareHeuristicScore {
	if !strings.EqualFold(strings.TrimSpace(oldNode.Role), strings.TrimSpace(newNode.Role)) {
		return compareHeuristicScore{}
	}

	score := compareHeuristicScore{}
	add := func(points int, reason string) {
		if points <= 0 {
			return
		}
		score.Score += points
		score.Reasons = append(score.Reasons, reason)
	}
	addSemantic := func(points int, reason string) {
		add(points, reason)
		score.Semantic = true
	}
	addStrong := func(points int, reason string) {
		addSemantic(points, reason)
		score.Strong = true
	}

	add(30, "same-role")
	if oldNode.Tag != "" && strings.EqualFold(oldNode.Tag, newNode.Tag) {
		add(20, "same-tag")
	}
	if oldNode.TestID != "" && oldNode.TestID == newNode.TestID {
		addStrong(100, "same-testid")
	}
	if oldNode.IDAttr != "" && oldNode.IDAttr == newNode.IDAttr {
		addStrong(90, "same-id")
	}
	if oldNode.Href != "" && oldNode.Href == newNode.Href {
		addStrong(90, "same-href")
	}
	if oldNode.NameAttr != "" && oldNode.NameAttr == newNode.NameAttr && oldNode.TypeAttr == newNode.TypeAttr {
		addSemantic(50, "same-name-attr-type")
	}
	if oldNode.Placeholder != "" && oldNode.Placeholder == newNode.Placeholder {
		addSemantic(40, "same-placeholder")
	}
	addStringScore(&score, oldNode.Name, newNode.Name, 40, 30, "name")
	addStringScore(&score, oldNode.Text, newNode.Text, 30, 25, "text")
	if compareNodeState(oldNode) == compareNodeState(newNode) {
		add(10, "same-state")
	}
	add(compareOriginalIndexScore(oldNode, newNode), "close-index")
	add(compareLayoutCenterScore(oldNode, newNode), "close-layout")

	if (!oldNode.Visible || !newNode.Visible) && !score.Strong {
		return compareHeuristicScore{}
	}
	if (compareHeuristicEmptyNode(oldNode) || compareHeuristicEmptyNode(newNode)) && !score.Strong {
		return compareHeuristicScore{}
	}
	return score
}

func addStringScore(score *compareHeuristicScore, oldValue string, newValue string, exactPoints int, similarPoints int, reason string) {
	oldValue = strings.TrimSpace(oldValue)
	newValue = strings.TrimSpace(newValue)
	if oldValue == "" || newValue == "" {
		return
	}
	if oldValue == newValue {
		score.Score += exactPoints
		score.Semantic = true
		score.Reasons = append(score.Reasons, "same-"+reason)
		return
	}
	similarity := compareStringSimilarity(oldValue, newValue)
	if similarity < 50 {
		return
	}
	points := similarity * similarPoints / 100
	if points <= 0 {
		return
	}
	score.Score += points
	score.Semantic = true
	score.Reasons = append(score.Reasons, "similar-"+reason)
}

func compareStringSimilarity(left string, right string) int {
	left = strings.ToLower(strings.TrimSpace(left))
	right = strings.ToLower(strings.TrimSpace(right))
	if left == "" || right == "" {
		return 0
	}
	if left == right {
		return 100
	}
	if strings.Contains(left, right) || strings.Contains(right, left) {
		return 70
	}

	leftTokens := strings.Fields(left)
	rightTokens := strings.Fields(right)
	if len(leftTokens) == 0 || len(rightTokens) == 0 {
		return 0
	}
	seen := map[string]struct{}{}
	for _, token := range leftTokens {
		seen[token] = struct{}{}
	}
	intersection := 0
	union := len(seen)
	for _, token := range rightTokens {
		if _, ok := seen[token]; ok {
			intersection++
			continue
		}
		union++
	}
	if union == 0 {
		return 0
	}
	return intersection * 100 / union
}

func compareOriginalIndexScore(oldNode compareSnapshotNode, newNode compareSnapshotNode) int {
	diff := compareAbs(oldNode.OriginalIndex - newNode.OriginalIndex)
	switch {
	case diff == 0:
		return 10
	case diff == 1:
		return 8
	case diff <= 3:
		return 5
	case diff <= 5:
		return 3
	default:
		return 0
	}
}

func compareLayoutCenterScore(oldNode compareSnapshotNode, newNode compareSnapshotNode) int {
	if oldNode.MatchBounds == nil || newNode.MatchBounds == nil {
		return 0
	}
	oldCenterX := oldNode.MatchBounds.X + oldNode.MatchBounds.W/2
	oldCenterY := oldNode.MatchBounds.Y + oldNode.MatchBounds.H/2
	newCenterX := newNode.MatchBounds.X + newNode.MatchBounds.W/2
	newCenterY := newNode.MatchBounds.Y + newNode.MatchBounds.H/2
	delta := max(compareAbs(oldCenterX-newCenterX), compareAbs(oldCenterY-newCenterY))
	switch {
	case delta <= 10:
		return 20
	case delta <= 25:
		return 15
	case delta <= 50:
		return 10
	case delta <= 100:
		return 5
	default:
		return 0
	}
}

func compareHeuristicEmptyNode(node compareSnapshotNode) bool {
	return strings.TrimSpace(strings.Join([]string{
		node.Name,
		node.Text,
		node.Value,
		node.TestID,
		node.IDAttr,
		node.Href,
		node.NameAttr,
		node.Placeholder,
	}, "")) == ""
}

func compareAllNodeIndices(nodes []compareSnapshotNode) []int {
	indices := make([]int, len(nodes))
	for i := range nodes {
		indices[i] = i
	}
	return indices
}

func compareNodeIndexSet(indices []int) map[int]struct{} {
	values := make(map[int]struct{}, len(indices))
	for _, index := range indices {
		values[index] = struct{}{}
	}
	return values
}

func compareSortedNodeIndexSet(values map[int]struct{}) []int {
	indices := make([]int, 0, len(values))
	for index := range values {
		indices = append(indices, index)
	}
	slices.Sort(indices)
	return indices
}

func compareUnmatchedNodeIndices(indices []int, matched map[int]struct{}) []int {
	unmatched := make([]int, 0, len(indices))
	for _, index := range indices {
		if _, ok := matched[index]; ok {
			continue
		}
		unmatched = append(unmatched, index)
	}
	slices.Sort(unmatched)
	return unmatched
}

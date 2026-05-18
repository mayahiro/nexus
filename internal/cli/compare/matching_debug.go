package comparecmd

import "strings"

func buildCompareMatchingDebug(mode string, oldNodes []compareSnapshotNode, newNodes []compareSnapshotNode, result compareNodeMatchResult) *compareMatchingDebug {
	return &compareMatchingDebug{
		Mode:                    mode,
		OldNodes:                len(oldNodes),
		NewNodes:                len(newNodes),
		MatchedNodes:            len(result.Matches),
		AmbiguousMatchesSkipped: result.AmbiguousSkipped,
		Matches:                 buildCompareMatchingDebugMatches(oldNodes, newNodes, result.Matches),
		UnmatchedOld:            buildCompareMatchingDebugNodes(oldNodes, result.UnmatchedOld, true),
		UnmatchedNew:            buildCompareMatchingDebugNodes(newNodes, result.UnmatchedNew, false),
		AmbiguousCandidates:     buildCompareMatchingDebugAmbiguousCandidates(oldNodes, newNodes, result.AmbiguousCandidates),
	}
}

func buildCompareMatchingDebugMatches(oldNodes []compareSnapshotNode, newNodes []compareSnapshotNode, matches []compareNodeMatch) []compareMatchingDebugMatch {
	if len(matches) == 0 {
		return nil
	}
	debug := make([]compareMatchingDebugMatch, 0, len(matches))
	for _, match := range matches {
		debug = append(debug, compareMatchingDebugMatch{
			Old:       buildCompareMatchingDebugNode(oldNodes, match.OldIndex, true),
			New:       buildCompareMatchingDebugNode(newNodes, match.NewIndex, false),
			MatchedBy: match.MatchedBy,
			Score:     match.Score,
			Reasons:   append([]string(nil), match.Reasons...),
		})
	}
	return debug
}

func buildCompareMatchingDebugAnchors(oldNodes []compareSnapshotNode, newNodes []compareSnapshotNode, anchors []compareHistogramAnchorCandidate) []compareMatchingDebugAnchor {
	if len(anchors) == 0 {
		return nil
	}
	debug := make([]compareMatchingDebugAnchor, 0, len(anchors))
	for _, anchor := range anchors {
		matchedBy := "histogram:" + anchor.Key.Kind
		reasons := []string{anchor.Key.Kind, "low-occurrence-anchor"}
		if strings.HasPrefix(anchor.Key.Kind, "decision:") {
			matchedBy = anchor.Key.Kind
			reasons = []string{"decision"}
		}
		debug = append(debug, compareMatchingDebugAnchor{
			Old:       buildCompareMatchingDebugNode(oldNodes, anchor.OldIndex, true),
			New:       buildCompareMatchingDebugNode(newNodes, anchor.NewIndex, false),
			KeyKind:   anchor.Key.Kind,
			KeyValue:  anchor.Key.Value,
			MatchedBy: matchedBy,
			Reasons:   reasons,
		})
	}
	return debug
}

func buildCompareMatchingDebugRegion(index int, oldNodes []compareSnapshotNode, newNodes []compareSnapshotNode, region compareHistogramRegion, exactMatches int, heuristicMatches int, ambiguousSkipped int) compareMatchingDebugRegion {
	oldStart, oldEnd := compareMatchingDebugOriginalRange(oldNodes, region.OldIndices)
	newStart, newEnd := compareMatchingDebugOriginalRange(newNodes, region.NewIndices)
	return compareMatchingDebugRegion{
		Index:                 index,
		OldStartOriginalIndex: oldStart,
		OldEndOriginalIndex:   oldEnd,
		OldNodeCount:          len(region.OldIndices),
		NewStartOriginalIndex: newStart,
		NewEndOriginalIndex:   newEnd,
		NewNodeCount:          len(region.NewIndices),
		ExactMatches:          exactMatches,
		HeuristicMatches:      heuristicMatches,
		AmbiguousSkipped:      ambiguousSkipped,
	}
}

func buildCompareMatchingDebugNodes(nodes []compareSnapshotNode, indices []int, oldSide bool) []compareMatchingDebugNode {
	if len(indices) == 0 {
		return nil
	}
	ordered := append([]int(nil), indices...)
	compareSortNodeIndicesBySequence(nodes, ordered)
	debug := make([]compareMatchingDebugNode, 0, len(ordered))
	for _, index := range ordered {
		debug = append(debug, buildCompareMatchingDebugNode(nodes, index, oldSide))
	}
	return debug
}

func buildCompareMatchingDebugAmbiguousCandidates(oldNodes []compareSnapshotNode, newNodes []compareSnapshotNode, candidates []compareAmbiguousCandidate) []compareMatchingDebugAmbiguousCandidate {
	if len(candidates) == 0 {
		return nil
	}
	debug := make([]compareMatchingDebugAmbiguousCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		options := make([]compareMatchingDebugCandidateOption, 0, len(candidate.NewCandidates))
		for _, option := range candidate.NewCandidates {
			options = append(options, compareMatchingDebugCandidateOption{
				Node:          buildCompareMatchingDebugNode(newNodes, option.NewIndex, false),
				Score:         option.Score,
				Reasons:       append([]string(nil), option.Reasons...),
				SharedKeys:    append([]string(nil), option.SharedKeys...),
				DifferingKeys: append([]string(nil), option.DifferingKeys...),
			})
		}
		debug = append(debug, compareMatchingDebugAmbiguousCandidate{
			Old:           buildCompareMatchingDebugNode(oldNodes, candidate.OldIndex, true),
			NewCandidates: options,
			Source:        candidate.Source,
			KeyKind:       candidate.KeyKind,
			KeyValue:      candidate.KeyValue,
			ReasonSkipped: candidate.ReasonSkipped,
		})
	}
	return debug
}

func buildCompareMatchingDebugNode(nodes []compareSnapshotNode, index int, oldSide bool) compareMatchingDebugNode {
	if index < 0 || index >= len(nodes) {
		return compareMatchingDebugNode{Index: index, OriginalIndex: -1}
	}
	node := nodes[index]
	var locator string
	if oldSide {
		locator = compareFindingLocator(&node, nil)
	} else {
		locator = compareFindingLocator(nil, &node)
	}
	return compareMatchingDebugNode{
		Index:            index,
		OriginalIndex:    node.OriginalIndex,
		Ref:              node.Ref,
		Locator:          locator,
		Selector:         compareNodeSelector(node, nodes),
		Role:             node.Role,
		Label:            node.Label,
		Name:             node.Name,
		Text:             node.Text,
		Href:             node.Href,
		TestID:           node.TestID,
		AriaLabel:        node.AriaLabel,
		Fingerprint:      node.Fingerprint,
		StructureKey:     node.StructureKey,
		SubtreeSignature: node.SubtreeSignature,
		Bounds:           node.MatchBounds,
	}
}

func compareMatchingDebugOriginalRange(nodes []compareSnapshotNode, indices []int) (int, int) {
	if len(indices) == 0 {
		return -1, -1
	}
	ordered := append([]int(nil), indices...)
	compareSortNodeIndicesBySequence(nodes, ordered)
	start := ordered[0]
	end := ordered[len(ordered)-1]
	if start < 0 || start >= len(nodes) || end < 0 || end >= len(nodes) {
		return -1, -1
	}
	return nodes[start].OriginalIndex, nodes[end].OriginalIndex
}

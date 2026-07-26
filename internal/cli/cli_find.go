package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	nagicli "github.com/mayahiro/nagicli-go"

	"github.com/mayahiro/nexus/internal/api"
)

func runFindRoleInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	return runFindInvocation(ctx, invocation, "role", nil, stdout, stderr)
}

func runFindTextInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	return runFindInvocation(ctx, invocation, "text", nil, stdout, stderr)
}

func runFindLabelInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	return runFindInvocation(ctx, invocation, "label", nil, stdout, stderr)
}

func runFindTestIDInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	return runFindInvocation(ctx, invocation, "testid", []string{"data-testid", "data-test"}, stdout, stderr)
}

func runFindHrefInvocation(ctx context.Context, invocation *nagicli.Invocation, stdout io.Writer, stderr io.Writer) int {
	return runFindInvocation(ctx, invocation, "href", []string{"href"}, stdout, stderr)
}

func runFindInvocation(
	ctx context.Context,
	invocation *nagicli.Invocation,
	kind string,
	attributes []string,
	stdout io.Writer,
	stderr io.Writer,
) int {
	sessionID := nagiStringValue(invocation, "session")
	asJSON := nagiBoolValue(invocation, "json")
	matchAll := nagiBoolValue(invocation, "all")
	nth := nagiIntValue(invocation, "nth")
	query := nagiStringValue(invocation, "query")
	action := nagiRawValue(invocation, "action")
	actionValue := nagiRawValue(invocation, "action-value")
	name := nagiStringValue(invocation, "name")

	client, err := connectClient(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	observation, err := observeTreeForFind(ctx, client, sessionID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	nodes := selectNodes(observation.Tree, func(node api.Node) bool {
		switch kind {
		case "role":
			if !strings.EqualFold(strings.TrimSpace(node.Role), strings.TrimSpace(query)) {
				return false
			}
			return name == "" || nodeMatches(node, name)
		case "text":
			return nodeMatches(node, query)
		case "label":
			if !node.Editable &&
				!node.Selectable &&
				!strings.EqualFold(node.Role, "textbox") &&
				!strings.EqualFold(node.Role, "combobox") {
				return false
			}
			return nodeMatches(node, query)
		default:
			needle := normalizeFindValue(query)
			for _, attribute := range attributes {
				if strings.Contains(normalizeFindValue(node.Attrs[attribute]), needle) {
					return true
				}
			}
			return false
		}
	})
	if matchAll {
		return renderFindMatches(nodes, asJSON, stdout, stderr)
	}

	matchQuery := query
	if kind == "role" && name != "" {
		matchQuery = name
	}
	node, err := chooseNode(nodes, matchQuery, nodeSelectionOptions{Nth: nth})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return executeFoundAction(ctx, client, sessionID, node, action, actionValue, asJSON, stdout, stderr)
}

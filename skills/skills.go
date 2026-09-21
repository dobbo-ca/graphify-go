// Package skills embeds the agent-host skill files graphify installs.
package skills

import _ "embed"

// Graphify is the Claude Code skill that teaches an agent to query the graph
// instead of grepping. Installed by `graphify install`.
//
//go:embed graphify/SKILL.md
var Graphify string

// Package deploy embeds deployment assets that Core serves to nodes.
package deploy

import _ "embed"

// AgentInstaller is deploy/install-agent.sh. Core serves it at
// /install-agent.sh so node enrollment does not depend on GitHub access
// (the repository can stay private).
//
//go:embed install-agent.sh
var AgentInstaller []byte

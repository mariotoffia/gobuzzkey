package envfile

import "strings"

// grokServer starts the buzz-grokbot MCP server straight from GitHub (the
// package is not on npm), pinned to a reviewed commit: the server runs with
// the agent key, so a later push to the repository must not run unreviewed.
// Move the pin only after reading the diff.
const grokServer = "npx -y github:davidorban/buzz-grokbot#4548c8b8dd4d0dd4b20c9641d80505297e80cdd1"

// renderGrok writes the part of the Grok Bot prompt that installs the MCP
// server with the identity. Values are unquoted: the bot would keep quotes as
// part of the value.
func renderGrok(vars [][2]string) []byte {
	var b strings.Builder
	b.WriteString("Add a custom MCP server (stdio, not a catalog plugin) named \"buzz\".\n")
	b.WriteString("Command: " + grokServer + "\n")
	b.WriteString("Environment variables:\n")
	for _, kv := range vars {
		b.WriteString("  " + kv[0] + " = " + kv[1] + "\n")
	}
	return []byte(b.String())
}

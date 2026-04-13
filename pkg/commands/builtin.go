package commands

// BuiltinDefinitions returns all built-in command definitions.
// Each command group is defined in its own cmd_*.go file.
// Definitions are stateless — runtime dependencies are provided
// via the Runtime parameter passed to handlers at execution time.
func BuiltinDefinitions() []Definition {
	// Keep the runtime command surface limited to the surviving root and chat
	// commands. Legacy root auth/model entrypoints are intentionally absent.
	return []Definition{
		startCommand(),
		helpCommand(),
		showCommand(),
		listCommand(),
		useCommand(),
		setCommand(),
		switchCommand(),
		checkCommand(),
		clearCommand(),
		fastCommand(),
		compactCommand(),
		statusCommand(),
		resetCommand(),
		subagentsCommand(),
		reloadCommand(),
	}
}

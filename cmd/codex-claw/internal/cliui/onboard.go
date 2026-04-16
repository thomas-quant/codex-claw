package cliui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// PrintOnboardComplete prints the post-onboard “ready” message and next steps.
func PrintOnboardComplete(logo string, encrypt bool, configPath, surface string, importedAuth bool) {
	if !UseFancyLayout() {
		printOnboardPlain(logo, encrypt, configPath, surface, importedAuth)
		return
	}
	printOnboardFancy(logo, encrypt, configPath, surface, importedAuth)
}

func printOnboardPlain(logo string, encrypt bool, configPath, surface string, importedAuth bool) {
	fmt.Printf("\n%s codex-claw is ready!\n", logo)
	fmt.Println("\nNext steps:")
	fmt.Println(indentForPlain(buildOnboardingSteps(encrypt, configPath, surface, importedAuth)))
	fmt.Println("")
	fmt.Println("     Recommended:")
	if importedAuth {
		fmt.Println("     - Managed live Codex auth is ready under `CODEX_CLAW_HOME/codex-home/auth.json`")
	} else {
		fmt.Println("     - Import existing Codex auth later with `codex-claw account import main --auth-file ~/.codex/auth.json`")
		fmt.Println("     - Or start `codex app-server` from a shell where Codex is already authenticated")
	}
	fmt.Println("     - If you enable DeepSeek fallback, export `DEEPSEEK_API_KEY` in that shell")
	fmt.Println("")
	fmt.Println("     See `docs/providers.md` for the current runtime options.")
	fmt.Println("")
	fmt.Println("  " + strings.ReplaceAll(chatStep(encrypt, surface), "\n", "\n  "))
}

func printOnboardFancy(logo string, encrypt bool, configPath, surface string, importedAuth bool) {
	inner := InnerWidth()
	box := borderStyle().MaxWidth(inner + 8)

	ready := titleBarStyle().Render(logo+" codex-claw is ready!") + "\n"
	fmt.Println()
	fmt.Println(box.Width(inner).Render(strings.TrimSpace(ready)))
	fmt.Println()

	steps := buildOnboardingSteps(encrypt, configPath, surface, importedAuth)
	rec := recommendedBlock(importedAuth)
	chat := chatStep(encrypt, surface)

	if UseColumnLayout() {
		leftW := min(inner/2-2, 52)
		rightW := inner - leftW - 4
		if rightW < 36 {
			rightW = 36
		}
		leftBlock := borderStyle().MaxWidth(leftW + 8).Width(leftW).
			Render(titleBarStyle().Render("Next steps") + "\n\n" + bodyStyle().Width(leftW).Render(steps))
		rightBlock := borderStyle().MaxWidth(rightW + 8).Width(rightW).
			Render(mutedStyle().Bold(true).Render("Recommended") + "\n\n" + bodyStyle().Width(rightW).Render(rec))
		gap := strings.Repeat(" ", 2)
		fmt.Println(lipgloss.JoinHorizontal(lipgloss.Top, leftBlock, gap, rightBlock))
		fmt.Println()
		full := borderStyle().Width(inner).Render(bodyStyle().Width(inner - 4).Render(chat))
		fmt.Println(full)
		return
	}

	// Same order as plain output: numbered steps → recommended → chat line.
	next := titleBarStyle().Render("Next steps") + "\n\n" +
		bodyStyle().Width(inner-4).Render(steps+"\n\n"+rec+"\n\n"+chat)
	fmt.Println(borderStyle().Width(inner).Render(next))
}

func buildOnboardingSteps(encrypt bool, configPath, surface string, importedAuth bool) string {
	var b strings.Builder
	step := 1
	if encrypt {
		b.WriteString("1. Set your encryption passphrase before starting codex-claw:\n")
		b.WriteString("   export CODEX_CLAW_KEY_PASSPHRASE=<your-passphrase>   # Linux/macOS\n")
		b.WriteString("   set CODEX_CLAW_KEY_PASSPHRASE=<your-passphrase>      # Windows cmd\n\n")
		step++
	}
	fmt.Fprintf(&b, "%d. Review your runtime settings in\n   %s\n\n", step, configPath)
	step++
	if surface != "" {
		fmt.Fprintf(&b, "%d. Finish your %s setup in .security.yml\n", step, surface)
		b.WriteString("   Add the matching bot token before starting the gateway.\n\n")
		step++
	}
	if importedAuth {
		fmt.Fprintf(&b, "%d. Managed live Codex auth imported into\n", step)
		b.WriteString("   CODEX_CLAW_HOME/codex-home/auth.json\n")
	}
	return b.String()
}

func recommendedBlock(importedAuth bool) string {
	if importedAuth {
		return "• The managed live Codex home already has an auth snapshot\n\n" +
			"• If you enable DeepSeek fallback, export `DEEPSEEK_API_KEY`\n\n" +
			"See `docs/providers.md` for the current runtime options."
	}
	return "• Import an existing Codex auth later with `codex-claw account import`\n\n" +
		"• Or start `codex app-server` from a shell where Codex is already authenticated\n\n" +
		"• If you enable DeepSeek fallback, export `DEEPSEEK_API_KEY`\n\n" +
		"See `docs/providers.md` for the current runtime options."
}

func chatStep(encrypt bool, surface string) string {
	step := 2
	if encrypt {
		step++
	}
	if surface != "" {
		step++
	}
	action := "Chat"
	if surface != "" {
		action = fmt.Sprintf("Open your %s chat", surface)
	}
	return fmt.Sprintf("%d. %s:\n   codex-claw agent -m \"Hello!\"", step, action)
}

func indentForPlain(text string) string {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = "  " + line
	}
	return strings.Join(lines, "\n")
}

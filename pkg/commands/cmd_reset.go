package commands

import "context"

func resetCommand() Definition {
	return Definition{
		Name:        "reset",
		Description: "Reset the current Codex thread",
		Usage:       "/reset",
		Handler: func(_ context.Context, req Request, rt *Runtime) error {
			if rt == nil || rt.ResetThread == nil {
				return req.Reply(unavailableMsg)
			}
			if err := runtimeResetThread(rt); err != nil {
				return req.Reply(err.Error())
			}
			return req.Reply("Thread reset.")
		},
	}
}

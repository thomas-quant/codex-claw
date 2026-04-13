package commands

import (
	"context"
	"fmt"
)

func setCommand() Definition {
	return Definition{
		Name:        "set",
		Description: "Set per-thread runtime settings",
		SubCommands: []SubCommand{
			{
				Name:        "model",
				Description: "Set the active model",
				ArgsUsage:   "<name>",
				Handler: func(_ context.Context, req Request, rt *Runtime) error {
					if rt == nil || (rt.SetModel == nil && rt.SwitchModel == nil) {
						return req.Reply(unavailableMsg)
					}
					value := nthToken(req.Text, 2)
					if value == "" {
						return req.Reply("Usage: /set model <name>")
					}
					oldModel, err := runtimeSetModel(rt, value)
					if err != nil {
						return req.Reply(err.Error())
					}
					if oldModel == "" {
						return req.Reply(fmt.Sprintf("Model set to %s", value))
					}
					return req.Reply(fmt.Sprintf("Model switched from %s to %s", oldModel, value))
				},
			},
			{
				Name:        "thinking",
				Description: "Set the active thinking mode",
				ArgsUsage:   "<mode>",
				Handler: func(_ context.Context, req Request, rt *Runtime) error {
					if rt == nil || rt.SetThinking == nil {
						return req.Reply(unavailableMsg)
					}
					value := nthToken(req.Text, 2)
					if value == "" {
						return req.Reply("Usage: /set thinking <mode>")
					}
					oldThinking, err := runtimeSetThinking(rt, value)
					if err != nil {
						return req.Reply(err.Error())
					}
					if oldThinking == "" {
						return req.Reply(fmt.Sprintf("Thinking mode set to %s", value))
					}
					return req.Reply(fmt.Sprintf("Thinking mode switched from %s to %s", oldThinking, value))
				},
			},
		},
	}
}

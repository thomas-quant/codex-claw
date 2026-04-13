package codexruntime

import "testing"

func TestProjector_FinalAssistantOnly(t *testing.T) {
	p := NewProjector("thr_1", "turn_1")

	p.Apply(Notification{
		Method: "item/reasoning/textDelta",
		Params: ReasoningTextDeltaParams{
			ThreadID: "thr_1",
			TurnID:   "turn_1",
			ItemID:   "reasoning_1",
			Text:     "hidden",
		},
	})
	p.Apply(Notification{
		Method: "item/agentMessage/delta",
		Params: AgentMessageDeltaParams{
			ThreadID: "thr_1",
			TurnID:   "turn_1",
			ItemID:   "msg_1",
			Delta:    "Hello",
		},
	})
	p.Apply(Notification{
		Method: "item/agentMessage/delta",
		Params: AgentMessageDeltaParams{
			ThreadID: "thr_1",
			TurnID:   "turn_1",
			ItemID:   "msg_1",
			Delta:    " world",
		},
	})

	if got := p.FinalAssistantText(); got != "Hello world" {
		t.Fatalf("FinalAssistantText() = %q, want %q", got, "Hello world")
	}
	if got := p.ReasoningText(); got != "hidden" {
		t.Fatalf("ReasoningText() = %q, want %q", got, "hidden")
	}
}

func TestProjector_UsesLastAssistantItemForVisibleOutput(t *testing.T) {
	p := NewProjector("thr_1", "turn_1")

	p.Apply(Notification{
		Method: "item/agentMessage/delta",
		Params: AgentMessageDeltaParams{
			ThreadID: "thr_1",
			TurnID:   "turn_1",
			ItemID:   "msg_progress",
			Delta:    "working",
		},
	})
	p.Apply(Notification{
		Method: "item/agentMessage/delta",
		Params: AgentMessageDeltaParams{
			ThreadID: "thr_1",
			TurnID:   "turn_1",
			ItemID:   "msg_final",
			Delta:    "done",
		},
	})

	if got := p.FinalAssistantText(); got != "done" {
		t.Fatalf("FinalAssistantText() = %q, want %q", got, "done")
	}
}

func TestProjector_PrefersLatestCompletedAssistantItem(t *testing.T) {
	p := NewProjector("thr_1", "turn_1")

	p.Apply(Notification{
		Method: "item/agentMessage/delta",
		Params: AgentMessageDeltaParams{
			ThreadID: "thr_1",
			TurnID:   "turn_1",
			ItemID:   "msg_progress",
			Delta:    "still working",
		},
	})
	p.Apply(Notification{
		Method: "item/agentMessage/delta",
		Params: AgentMessageDeltaParams{
			ThreadID: "thr_1",
			TurnID:   "turn_1",
			ItemID:   "msg_final",
			Delta:    "final",
		},
	})
	p.Apply(Notification{
		Method: "item/completed",
		Params: ItemCompletedParams{
			ThreadID: "thr_1",
			TurnID:   "turn_1",
			Item: OutputItem{
				ID:     "msg_final",
				Type:   ItemTypeAgentMessage,
				Role:   ItemRoleAssistant,
				Status: ItemStatusCompleted,
			},
		},
	})
	p.Apply(Notification{
		Method: "item/agentMessage/delta",
		Params: AgentMessageDeltaParams{
			ThreadID: "thr_1",
			TurnID:   "turn_1",
			ItemID:   "msg_after",
			Delta:    "typing noise",
		},
	})

	if got := p.FinalAssistantText(); got != "final" {
		t.Fatalf("FinalAssistantText() = %q, want %q", got, "final")
	}
}

func TestProjector_IgnoresNonAssistantCompletedAndProgressChatter(t *testing.T) {
	p := NewProjector("thr_1", "turn_1")

	p.Apply(Notification{
		Method: "turn/progress",
		Params: map[string]any{
			"thread_id": "thr_1",
			"turn_id":   "turn_1",
			"message":   "thinking",
		},
	})
	p.Apply(Notification{
		Method: "item/reasoning/textDelta",
		Params: ReasoningTextDeltaParams{
			ThreadID: "thr_1",
			TurnID:   "turn_1",
			ItemID:   "reasoning_1",
			Text:     "hidden",
		},
	})
	p.Apply(Notification{
		Method: "item/completed",
		Params: ItemCompletedParams{
			ThreadID: "thr_1",
			TurnID:   "turn_1",
			Item: OutputItem{
				ID:     "reasoning_1",
				Type:   ItemTypeReasoning,
				Status: ItemStatusCompleted,
			},
		},
	})
	p.Apply(Notification{
		Method: "item/agentMessage/delta",
		Params: AgentMessageDeltaParams{
			ThreadID: "thr_1",
			TurnID:   "turn_1",
			ItemID:   "msg_1",
			Delta:    "visible",
		},
	})

	if got := p.FinalAssistantText(); got != "visible" {
		t.Fatalf("FinalAssistantText() = %q, want %q", got, "visible")
	}
	if got := p.ReasoningText(); got != "hidden" {
		t.Fatalf("ReasoningText() = %q, want %q", got, "hidden")
	}
}

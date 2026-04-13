package codexruntime

import "strings"

type projectedAssistantItem struct {
	text      string
	completed bool
}

type Projector struct {
	threadID string
	turnID   string

	assistantByItem    map[string]*projectedAssistantItem
	assistantOrder     []string
	completedAssistant []string
	reasoning          strings.Builder
}

func NewProjector(threadID, turnID string) *Projector {
	return &Projector{
		threadID:           threadID,
		turnID:             turnID,
		assistantByItem:    make(map[string]*projectedAssistantItem),
		completedAssistant: make([]string, 0),
	}
}

func (p *Projector) Apply(notification Notification) {
	switch params := notification.Params.(type) {
	case AgentMessageDeltaParams:
		p.applyAgentMessageDelta(notification.Method, params)
	case *AgentMessageDeltaParams:
		if params != nil {
			p.applyAgentMessageDelta(notification.Method, *params)
		}
	case ReasoningTextDeltaParams:
		p.applyReasoningTextDelta(notification.Method, params)
	case *ReasoningTextDeltaParams:
		if params != nil {
			p.applyReasoningTextDelta(notification.Method, *params)
		}
	case ItemCompletedParams:
		p.applyItemCompleted(notification.Method, params)
	case *ItemCompletedParams:
		if params != nil {
			p.applyItemCompleted(notification.Method, *params)
		}
	}
}

func (p *Projector) FinalAssistantText() string {
	if len(p.completedAssistant) > 0 {
		itemID := p.completedAssistant[len(p.completedAssistant)-1]
		if item := p.assistantByItem[itemID]; item != nil {
			return item.text
		}
	}
	if len(p.assistantOrder) == 0 {
		return ""
	}
	itemID := p.assistantOrder[len(p.assistantOrder)-1]
	if item := p.assistantByItem[itemID]; item != nil {
		return item.text
	}
	return ""
}

func (p *Projector) ReasoningText() string {
	return p.reasoning.String()
}

func (p *Projector) applyAgentMessageDelta(method string, params AgentMessageDeltaParams) {
	if method != MethodItemAgentMessageDelta {
		return
	}
	if params.ThreadID != p.threadID || params.TurnID != p.turnID || params.ItemID == "" || params.Delta == "" {
		return
	}

	item := p.assistantItem(params.ItemID)
	item.text += params.Delta
}

func (p *Projector) applyReasoningTextDelta(method string, params ReasoningTextDeltaParams) {
	if method != MethodItemReasoningTextDelta {
		return
	}
	if params.ThreadID != p.threadID || params.TurnID != p.turnID || params.Text == "" {
		return
	}

	p.reasoning.WriteString(params.Text)
}

func (p *Projector) applyItemCompleted(method string, params ItemCompletedParams) {
	if method != MethodItemCompleted {
		return
	}
	if params.ThreadID != p.threadID || params.TurnID != p.turnID || params.Item.ID == "" {
		return
	}
	if !isAssistantItem(params.Item) {
		return
	}

	item := p.assistantItem(params.Item.ID)
	if params.Item.Text != "" {
		item.text = params.Item.Text
	}
	if item.completed {
		return
	}

	item.completed = true
	p.completedAssistant = append(p.completedAssistant, params.Item.ID)
}

func (p *Projector) assistantItem(itemID string) *projectedAssistantItem {
	if item, ok := p.assistantByItem[itemID]; ok {
		return item
	}

	item := &projectedAssistantItem{}
	p.assistantByItem[itemID] = item
	p.assistantOrder = append(p.assistantOrder, itemID)
	return item
}

func isAssistantItem(item OutputItem) bool {
	return normalizeLifecycleValue(item.Role) == normalizeLifecycleValue(ItemRoleAssistant) ||
		normalizeLifecycleValue(item.Type) == normalizeLifecycleValue(ItemTypeAgentMessage)
}

func normalizeLifecycleValue(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "_", "")
	return strings.ReplaceAll(value, "-", "")
}

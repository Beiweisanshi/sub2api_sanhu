package service

import "strings"

const (
	defaultOpenAIMessagesDispatchOpusMappedModel   = "gpt-5.4"
	defaultOpenAIMessagesDispatchSonnetMappedModel = "gpt-5.3-codex"
	defaultOpenAIMessagesDispatchHaikuMappedModel  = "gpt-5.4-mini"
)

type OpenAIMessagesDispatchModelResolution struct {
	Model           string
	ReasoningEffort string
}

func normalizeOpenAIMessagesDispatchMappedModel(model string) string {
	return strings.TrimSpace(model)
}

func resolveOpenAIMessagesDispatchMappedTarget(target string) OpenAIMessagesDispatchModelResolution {
	target = normalizeOpenAIMessagesDispatchMappedModel(target)
	if target == "" {
		return OpenAIMessagesDispatchModelResolution{}
	}

	normalizedModel, reasoningEffort := normalizeOpenAICompatResolvedModel(target)
	if normalizedModel != target {
		return OpenAIMessagesDispatchModelResolution{
			Model:           normalizedModel,
			ReasoningEffort: reasoningEffort,
		}
	}

	return OpenAIMessagesDispatchModelResolution{Model: target}
}

func normalizeOpenAIMessagesDispatchModelConfig(cfg OpenAIMessagesDispatchModelConfig) OpenAIMessagesDispatchModelConfig {
	out := OpenAIMessagesDispatchModelConfig{
		OpusMappedModel:   normalizeOpenAIMessagesDispatchMappedModel(cfg.OpusMappedModel),
		SonnetMappedModel: normalizeOpenAIMessagesDispatchMappedModel(cfg.SonnetMappedModel),
		HaikuMappedModel:  normalizeOpenAIMessagesDispatchMappedModel(cfg.HaikuMappedModel),
	}

	if len(cfg.ExactModelMappings) > 0 {
		out.ExactModelMappings = make(map[string]string, len(cfg.ExactModelMappings))
		for requestedModel, mappedModel := range cfg.ExactModelMappings {
			requestedModel = strings.TrimSpace(requestedModel)
			mappedModel = normalizeOpenAIMessagesDispatchMappedModel(mappedModel)
			if requestedModel == "" || mappedModel == "" {
				continue
			}
			out.ExactModelMappings[requestedModel] = mappedModel
		}
		if len(out.ExactModelMappings) == 0 {
			out.ExactModelMappings = nil
		}
	}

	return out
}

func claudeMessagesDispatchFamily(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if !strings.HasPrefix(normalized, "claude") {
		return ""
	}
	switch {
	case strings.Contains(normalized, "opus"):
		return "opus"
	case strings.Contains(normalized, "sonnet"):
		return "sonnet"
	case strings.Contains(normalized, "haiku"):
		return "haiku"
	default:
		return ""
	}
}

func (g *Group) ResolveMessagesDispatchModel(requestedModel string) string {
	return g.ResolveMessagesDispatchModelWithReasoning(requestedModel).Model
}

func (g *Group) ResolveMessagesDispatchModelWithReasoning(requestedModel string) OpenAIMessagesDispatchModelResolution {
	if g == nil {
		return OpenAIMessagesDispatchModelResolution{}
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return OpenAIMessagesDispatchModelResolution{}
	}

	cfg := normalizeOpenAIMessagesDispatchModelConfig(g.MessagesDispatchModelConfig)
	if mappedModel := strings.TrimSpace(cfg.ExactModelMappings[requestedModel]); mappedModel != "" {
		return resolveOpenAIMessagesDispatchMappedTarget(mappedModel)
	}

	switch claudeMessagesDispatchFamily(requestedModel) {
	case "opus":
		if mappedModel := strings.TrimSpace(cfg.OpusMappedModel); mappedModel != "" {
			return resolveOpenAIMessagesDispatchMappedTarget(mappedModel)
		}
		return resolveOpenAIMessagesDispatchMappedTarget(defaultOpenAIMessagesDispatchOpusMappedModel)
	case "sonnet":
		if mappedModel := strings.TrimSpace(cfg.SonnetMappedModel); mappedModel != "" {
			return resolveOpenAIMessagesDispatchMappedTarget(mappedModel)
		}
		return resolveOpenAIMessagesDispatchMappedTarget(defaultOpenAIMessagesDispatchSonnetMappedModel)
	case "haiku":
		if mappedModel := strings.TrimSpace(cfg.HaikuMappedModel); mappedModel != "" {
			return resolveOpenAIMessagesDispatchMappedTarget(mappedModel)
		}
		return resolveOpenAIMessagesDispatchMappedTarget(defaultOpenAIMessagesDispatchHaikuMappedModel)
	default:
		return OpenAIMessagesDispatchModelResolution{}
	}
}

func sanitizeGroupMessagesDispatchFields(g *Group) {
	if g == nil || g.Platform == PlatformOpenAI {
		return
	}
	g.AllowMessagesDispatch = false
	g.DefaultMappedModel = ""
	g.MessagesDispatchModelConfig = OpenAIMessagesDispatchModelConfig{}
}

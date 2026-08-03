// Package classifier decides which memory layer an entry belongs to.
//
// Design: rule pre-screen + LLM re-review (方案 C, recommended).
//
// Three layers:
//   - "working"     -> PICO_SESSION (表意识, session-scoped, no vector/encryption)
//   - "deep"        -> PICO_MEMORY_DEEP (个人深藏潜意识, encrypted + vector)
//   - "shared"      -> PICO_MEMORY_SHARED (集体潜意识, plaintext + vector)
package classifier

import (
	"context"
	"strings"
)

// Layer is the destination layer for a memory entry.
type Layer string

const (
	LayerWorking Layer = "working"
	LayerDeep    Layer = "deep"
	LayerShared  Layer = "shared"
)

// Decision is the result of classification.
type Decision struct {
	Layer     Layer
	Confidence float64 // 0..1
	Reason    string
}

// RuleSet is a configurable keyword/pattern-based classifier.
type RuleSet struct {
	// SharedKeywords: if content contains any of these, force LayerShared
	SharedKeywords []string
	// DeepKeywords: if content contains any of these, force LayerDeep
	DeepKeywords []string
	// SharedSources: agent names that always produce shared memory
	SharedSources []string
	// DeepSources: agent names that always produce deep memory
	DeepSources []string
}

// DefaultRuleSet returns a reasonable starting configuration.
func DefaultRuleSet() *RuleSet {
	return &RuleSet{
		SharedKeywords: []string{
			"all agents", "collective", "shared", "prototype", "集体",
			"原型", "共识", "protocol", "standard", "rule", "铁律",
		},
		DeepKeywords: []string{
			"secret", "private", "博弈", "conflict", "compete", "strategy",
			"threat", "vulnerability", "personal", "记忆", "决策",
		},
		SharedSources: []string{}, // filled at runtime
		DeepSources:   []string{}, // filled at runtime
	}
}

// Classify runs rule-based pre-screen on a memory entry.
func (r *RuleSet) Classify(agentName, content string, metadata map[string]any) *Decision {
	// 1. Source-based rule
	for _, s := range r.SharedSources {
		if strings.EqualFold(agentName, s) {
			return &Decision{Layer: LayerShared, Confidence: 0.95, Reason: "source rule: shared"}
		}
	}
	for _, s := range r.DeepSources {
		if strings.EqualFold(agentName, s) {
			return &Decision{Layer: LayerDeep, Confidence: 0.95, Reason: "source rule: deep"}
		}
	}

	// 2. Keyword rule
	lc := strings.ToLower(content)
	for _, kw := range r.SharedKeywords {
		if strings.Contains(lc, strings.ToLower(kw)) {
			return &Decision{Layer: LayerShared, Confidence: 0.7, Reason: "keyword: " + kw}
		}
	}
	for _, kw := range r.DeepKeywords {
		if strings.Contains(lc, strings.ToLower(kw)) {
			return &Decision{Layer: LayerDeep, Confidence: 0.7, Reason: "keyword: " + kw}
		}
	}

	// 3. Default: deep (personal memory is the safest default)
	return &Decision{Layer: LayerDeep, Confidence: 0.5, Reason: "default"}
}

// LLMReReview sends the rule decision + content to LLM for final confirmation.
// Returns the confirmed decision (or overrides if LLM disagrees).
//
// This is where the "count me in" protocol lives — when the classifier is
// uncertain (< 0.7 confidence), the question bubbles up to the user.
func (r *RuleSet) LLMReReview(ctx context.Context, _ context.Context, rule *Decision, content string) *Decision {
	// TODO: wire to LiteLLM/OpenAI-compatible endpoint
	// For now, return the rule decision as-is.
	return rule
}

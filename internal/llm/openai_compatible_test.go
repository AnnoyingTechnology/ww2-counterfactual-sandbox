package llm

import "testing"

func TestTrimJSONEnvelopeStripsCodeFences(t *testing.T) {
	input := "```json\n{\"ok\":true}\n```"
	got := trimJSONEnvelope(input)
	if got != "{\"ok\":true}" {
		t.Fatalf("unexpected trimmed output: %q", got)
	}
}

func TestTrimJSONEnvelopeStripsThinkBlockBeforeJSON(t *testing.T) {
	input := "<think>\nI should think first.\n</think>\n{\"ok\":true}"
	got := trimJSONEnvelope(input)
	if got != "{\"ok\":true}" {
		t.Fatalf("unexpected trimmed output: %q", got)
	}
}

func TestTrimJSONEnvelopeStripsThinkBlockInsideFences(t *testing.T) {
	input := "```json\n<think>hidden reasoning</think>\n{\"ok\":true}\n```"
	got := trimJSONEnvelope(input)
	if got != "{\"ok\":true}" {
		t.Fatalf("unexpected trimmed output: %q", got)
	}
}

func TestStripReasoningBlocksSupportsThinkingTag(t *testing.T) {
	input := "<thinking>step by step</thinking>{\"ok\":true}"
	got := stripReasoningBlocks(input)
	if got != "{\"ok\":true}" {
		t.Fatalf("unexpected stripped output: %q", got)
	}
}

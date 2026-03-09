package interview

import (
	"strings"
	"testing"
)

func TestBuildClaudeMessagesEvaluationTurnIncludesHistoryCurrentQuestionAndAnswer(t *testing.T) {
	turnCtx := &AITurnContext{
		PreferredLanguage:  "es",
		CurrentAreaSlug:    "protected_ground",
		CurrentAreaID:      1,
		CurrentAreaLabel:   "Protected ground",
		Description:        "Describe the protected ground.",
		SufficiencyReqs:    "Name the protected ground and why it applies.",
		AreaStatus:         AreaStatusInProgress,
		FollowUpsRemaining: 3,
		TimeRemainingS:     1200,
		QuestionsRemaining: 18,
		CriteriaRemaining:  5,
		CriteriaCoverage: []CriteriaCoverage{
			{ID: 1, Name: "protected_ground", Status: AreaStatusInProgress},
			{ID: 2, Name: "well_founded_fear", Status: AreaStatusPending},
		},
		HistoryTurns: []HistoryTurn{
			{QuestionText: "What happened to you?", AnswerText: "I was threatened."},
		},
		CurrentQuestionText: "Why do you think they targeted you?",
		LatestAnswerText:    "Because of my political opinion.",
	}

	messages, turnUserMessage := buildClaudeMessages(turnCtx, "Ask one last focused question.", "Opening instruction")

	if len(messages) != 4 {
		t.Fatalf("len(messages) = %d, want 4", len(messages))
	}
	if messages[0]["role"] != "assistant" || messages[1]["role"] != "user" {
		t.Fatalf("history roles = [%v %v], want [assistant user]", messages[0]["role"], messages[1]["role"])
	}
	if messages[2]["role"] != "assistant" {
		t.Fatalf("messages[2].role = %v, want assistant", messages[2]["role"])
	}
	if messages[3]["role"] != "user" {
		t.Fatalf("messages[3].role = %v, want user", messages[3]["role"])
	}
	if got := messageText(t, messages[2]); got != "Why do you think they targeted you?" {
		t.Fatalf("current assistant question = %q, want latest question", got)
	}
	if !containsAll(turnUserMessage,
		"<mode>evaluate</mode>",
		"<priority>Ask one last focused question.</priority>",
		"<current_question>Why do you think they targeted you?</current_question>",
		"<candidate_answer>Because of my political opinion.</candidate_answer>",
	) {
		t.Fatalf("turn user message missing expected XML: %s", turnUserMessage)
	}
}

func TestBuildClaudeMessagesOpeningTurnUsesOpeningInstruction(t *testing.T) {
	turnCtx := &AITurnContext{
		PreferredLanguage:  "en",
		IsOpeningTurn:      true,
		CurrentAreaSlug:    "open_floor",
		CurrentAreaID:      7,
		CurrentAreaLabel:   "Open floor",
		Description:        "Anything else to share.",
		SufficiencyReqs:    "Always sufficient after response.",
		AreaStatus:         AreaStatusInProgress,
		FollowUpsRemaining: 6,
		TimeRemainingS:     300,
		QuestionsRemaining: 5,
		CriteriaRemaining:  1,
		CriteriaCoverage: []CriteriaCoverage{
			{ID: 7, Name: "open_floor", Status: AreaStatusInProgress},
		},
	}

	messages, turnUserMessage := buildClaudeMessages(turnCtx, "", "Set evaluation to null.")

	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	if messages[0]["role"] != "user" {
		t.Fatalf("messages[0].role = %v, want user", messages[0]["role"])
	}
	if !containsAll(turnUserMessage,
		"<mode>opening</mode>",
		"<instruction>Set evaluation to null.</instruction>",
	) {
		t.Fatalf("opening turn message missing expected XML: %s", turnUserMessage)
	}
}

func TestApplyClaudePromptCachingMarksLastReusableBlock(t *testing.T) {
	systemBlocks := buildClaudeSystemBlocks("system prompt")
	messages := []map[string]interface{}{
		newClaudeTextMessage("assistant", "Q1"),
		newClaudeTextMessage("user", "A1"),
		newClaudeTextMessage("assistant", "Q2"),
		newClaudeTextMessage("user", "dynamic"),
	}

	applyClaudePromptCaching(systemBlocks, messages, &AITurnContext{}, true)

	if got := cacheControlType(messages[2]); got != "ephemeral" {
		t.Fatalf("cache_control on last reusable block = %q, want ephemeral", got)
	}
}

func TestApplyClaudePromptCachingFallsBackToSystemOnFirstOpeningTurn(t *testing.T) {
	systemBlocks := buildClaudeSystemBlocks("system prompt")
	messages := []map[string]interface{}{newClaudeTextMessage("user", "opening")}

	applyClaudePromptCaching(systemBlocks, messages, &AITurnContext{IsOpeningTurn: true}, true)

	if got, _ := systemBlocks[0]["cache_control"].(map[string]string); got["type"] != "ephemeral" {
		t.Fatalf("system block cache_control = %#v, want ephemeral", systemBlocks[0]["cache_control"])
	}
}

func messageText(t *testing.T, message map[string]interface{}) string {
	t.Helper()
	content, ok := message["content"].([]map[string]interface{})
	if !ok || len(content) == 0 {
		t.Fatalf("message content = %#v, want content block array", message["content"])
	}
	text, _ := content[0]["text"].(string)
	return text
}

func cacheControlType(message map[string]interface{}) string {
	content, ok := message["content"].([]map[string]interface{})
	if !ok || len(content) == 0 {
		return ""
	}
	cacheControl, ok := content[len(content)-1]["cache_control"].(map[string]string)
	if !ok {
		return ""
	}
	return cacheControl["type"]
}

func containsAll(s string, substrings ...string) bool {
	for _, substring := range substrings {
		if !strings.Contains(s, substring) {
			return false
		}
	}
	return true
}

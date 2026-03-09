package interview

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"log/slog"
	"strings"
	"text/template"
)

type turnPromptCoverageData struct {
	ID     int
	Name   string
	Status string
}

type turnPromptData struct {
	Mode                  string
	CurrentAreaID         int
	CurrentAreaSlug       string
	CurrentAreaLabel      string
	Description           string
	SufficiencyReqs       string
	AreaStatus            string
	IsPreAddressed        bool
	FollowUpsRemaining    int
	TimeRemainingS        int
	QuestionsRemaining    int
	CriteriaRemaining     int
	RequiredLanguageLabel string
	RequiredLanguageCode  string
	CriteriaCoverage      []turnPromptCoverageData
	Priority              string
	CurrentQuestionText   string
	LatestAnswerText      string
	Instruction           string
}

var (
	openingTurnUserMessageTemplate = template.Must(
		template.New("opening_turn_user_message").
			Funcs(template.FuncMap{"xml": xmlEscape}).
			Parse(`<turn>
  <mode>{{.Mode}}</mode>
  <criterion>
    <id>{{.CurrentAreaID}}</id>
    <slug>{{xml .CurrentAreaSlug}}</slug>
    <label>{{xml .CurrentAreaLabel}}</label>
    <description>{{xml .Description}}</description>
    <sufficiency_requirements>{{xml .SufficiencyReqs}}</sufficiency_requirements>
  </criterion>
  <state>
    <area_status>{{xml .AreaStatus}}</area_status>
    <is_pre_addressed>{{.IsPreAddressed}}</is_pre_addressed>
    <follow_ups_remaining>{{.FollowUpsRemaining}}</follow_ups_remaining>
  </state>
  <progress>
    <time_remaining_s>{{.TimeRemainingS}}</time_remaining_s>
    <questions_remaining>{{.QuestionsRemaining}}</questions_remaining>
    <criteria_remaining>{{.CriteriaRemaining}}</criteria_remaining>
    <required_language>
      <label>{{xml .RequiredLanguageLabel}}</label>
      <code>{{xml .RequiredLanguageCode}}</code>
    </required_language>
  </progress>
  <criteria_coverage>{{range .CriteriaCoverage}}
    <area>
      <id>{{.ID}}</id>
      <name>{{xml .Name}}</name>
      <status>{{xml .Status}}</status>
    </area>{{end}}
  </criteria_coverage>{{if .Priority}}
  <priority>{{xml .Priority}}</priority>{{end}}
  <instruction>{{xml .Instruction}}</instruction>
</turn>`),
	)
	evaluationTurnUserMessageTemplate = template.Must(
		template.New("evaluation_turn_user_message").
			Funcs(template.FuncMap{"xml": xmlEscape}).
			Parse(`<turn>
  <mode>{{.Mode}}</mode>
  <criterion>
    <id>{{.CurrentAreaID}}</id>
    <slug>{{xml .CurrentAreaSlug}}</slug>
    <label>{{xml .CurrentAreaLabel}}</label>
    <description>{{xml .Description}}</description>
    <sufficiency_requirements>{{xml .SufficiencyReqs}}</sufficiency_requirements>
  </criterion>
  <state>
    <area_status>{{xml .AreaStatus}}</area_status>
    <is_pre_addressed>{{.IsPreAddressed}}</is_pre_addressed>
    <follow_ups_remaining>{{.FollowUpsRemaining}}</follow_ups_remaining>
  </state>
  <progress>
    <time_remaining_s>{{.TimeRemainingS}}</time_remaining_s>
    <questions_remaining>{{.QuestionsRemaining}}</questions_remaining>
    <criteria_remaining>{{.CriteriaRemaining}}</criteria_remaining>
    <required_language>
      <label>{{xml .RequiredLanguageLabel}}</label>
      <code>{{xml .RequiredLanguageCode}}</code>
    </required_language>
  </progress>
  <criteria_coverage>{{range .CriteriaCoverage}}
    <area>
      <id>{{.ID}}</id>
      <name>{{xml .Name}}</name>
      <status>{{xml .Status}}</status>
    </area>{{end}}
  </criteria_coverage>{{if .Priority}}
  <priority>{{xml .Priority}}</priority>{{end}}
  <current_question>{{xml .CurrentQuestionText}}</current_question>
  <candidate_answer>{{xml .LatestAnswerText}}</candidate_answer>
  <instruction>{{xml .Instruction}}</instruction>
</turn>`),
	)
)

func buildClaudeSystemBlocks(systemPrompt string) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type": "text",
			"text": systemPrompt,
		},
	}
}

func buildClaudeMessages(turnCtx *AITurnContext, priorityPrompt, openingTurnPrompt string) ([]map[string]interface{}, string) {
	turnUserMessage := buildTurnUserMessage(turnCtx, priorityPrompt, openingTurnPrompt)
	messages := make([]map[string]interface{}, 0, len(turnCtx.HistoryTurns)*2+2)
	for _, turn := range turnCtx.HistoryTurns {
		messages = append(messages,
			newClaudeTextMessage("assistant", turn.QuestionText),
			newClaudeTextMessage("user", turn.AnswerText),
		)
	}
	if !turnCtx.IsOpeningTurn {
		messages = append(messages, newClaudeTextMessage("assistant", turnCtx.CurrentQuestionText))
	}
	messages = append(messages, newClaudeTextMessage("user", turnUserMessage))
	return messages, turnUserMessage
}

func buildOllamaMessages(turnCtx *AITurnContext, priorityPrompt, openingTurnPrompt string) ([]map[string]interface{}, string) {
	turnUserMessage := buildTurnUserMessage(turnCtx, priorityPrompt, openingTurnPrompt)
	messages := make([]map[string]interface{}, 0, len(turnCtx.HistoryTurns)*2+2)
	for _, turn := range turnCtx.HistoryTurns {
		messages = append(messages,
			map[string]interface{}{"role": "assistant", "content": turn.QuestionText},
			map[string]interface{}{"role": "user", "content": turn.AnswerText},
		)
	}
	if !turnCtx.IsOpeningTurn {
		messages = append(messages, map[string]interface{}{"role": "assistant", "content": turnCtx.CurrentQuestionText})
	}
	messages = append(messages, map[string]interface{}{"role": "user", "content": turnUserMessage})
	return messages, turnUserMessage
}

func applyClaudePromptCaching(systemBlocks []map[string]interface{}, messages []map[string]interface{}, turnCtx *AITurnContext, enabled bool) {
	if !enabled {
		return
	}

	switch {
	case !turnCtx.IsOpeningTurn && len(messages) >= 2:
		addCacheControlToClaudeMessage(messages[len(messages)-2])
	case len(turnCtx.HistoryTurns) > 0 && len(messages) >= 2:
		addCacheControlToClaudeMessage(messages[len(messages)-2])
	case len(systemBlocks) > 0:
		systemBlocks[len(systemBlocks)-1]["cache_control"] = map[string]string{"type": "ephemeral"}
	}
}

func buildTurnUserMessage(turnCtx *AITurnContext, priorityPrompt, openingTurnPrompt string) string {
	requiredLanguageCode, requiredLanguageLabel := requiredLanguageInfo(turnCtx.PreferredLanguage)
	coverage := make([]turnPromptCoverageData, 0, len(turnCtx.CriteriaCoverage))
	for _, area := range turnCtx.CriteriaCoverage {
		coverage = append(coverage, turnPromptCoverageData{
			ID:     area.ID,
			Name:   area.Name,
			Status: string(area.Status),
		})
	}

	data := turnPromptData{
		CurrentAreaID:         turnCtx.CurrentAreaID,
		CurrentAreaSlug:       turnCtx.CurrentAreaSlug,
		CurrentAreaLabel:      turnCtx.CurrentAreaLabel,
		Description:           turnCtx.Description,
		SufficiencyReqs:       turnCtx.SufficiencyReqs,
		AreaStatus:            string(turnCtx.AreaStatus),
		IsPreAddressed:        turnCtx.IsPreAddressed,
		FollowUpsRemaining:    turnCtx.FollowUpsRemaining,
		TimeRemainingS:        turnCtx.TimeRemainingS,
		QuestionsRemaining:    turnCtx.QuestionsRemaining,
		CriteriaRemaining:     turnCtx.CriteriaRemaining,
		RequiredLanguageLabel: requiredLanguageLabel,
		RequiredLanguageCode:  requiredLanguageCode,
		CriteriaCoverage:      coverage,
		Priority:              strings.TrimSpace(priorityPrompt),
		CurrentQuestionText:   turnCtx.CurrentQuestionText,
		LatestAnswerText:      turnCtx.LatestAnswerText,
	}

	var (
		tmpl *template.Template
	)
	if turnCtx.IsOpeningTurn {
		data.Mode = "opening"
		data.Instruction = firstNonEmpty(
			strings.TrimSpace(openingTurnPrompt),
			"This is an opening turn for the current criterion. Set evaluation to null and generate an opening question in the required language.",
		)
		tmpl = openingTurnUserMessageTemplate
	} else {
		data.Mode = "evaluate"
		data.Instruction = fmt.Sprintf(
			"Evaluate only the candidate_answer against the current criterion. Return next_question strictly in %s (%s). Do not switch languages.",
			requiredLanguageLabel,
			requiredLanguageCode,
		)
		tmpl = evaluationTurnUserMessageTemplate
	}

	var b bytes.Buffer
	if err := tmpl.Execute(&b, data); err != nil {
		slog.Warn("failed to render interview turn XML prompt; using fallback", "error", err)
		return buildTurnUserMessageFallback(data)
	}
	return b.String()
}

func buildTurnUserMessageFallback(data turnPromptData) string {
	var b strings.Builder
	b.WriteString("<turn>\n")
	b.WriteString("  <mode>" + xmlEscape(data.Mode) + "</mode>\n")
	b.WriteString("  <criterion>\n")
	b.WriteString(fmt.Sprintf("    <id>%d</id>\n", data.CurrentAreaID))
	b.WriteString("    <slug>" + xmlEscape(data.CurrentAreaSlug) + "</slug>\n")
	b.WriteString("    <label>" + xmlEscape(data.CurrentAreaLabel) + "</label>\n")
	b.WriteString("    <description>" + xmlEscape(data.Description) + "</description>\n")
	b.WriteString("    <sufficiency_requirements>" + xmlEscape(data.SufficiencyReqs) + "</sufficiency_requirements>\n")
	b.WriteString("  </criterion>\n")
	b.WriteString("  <state>\n")
	b.WriteString("    <area_status>" + xmlEscape(data.AreaStatus) + "</area_status>\n")
	b.WriteString(fmt.Sprintf("    <is_pre_addressed>%t</is_pre_addressed>\n", data.IsPreAddressed))
	b.WriteString(fmt.Sprintf("    <follow_ups_remaining>%d</follow_ups_remaining>\n", data.FollowUpsRemaining))
	b.WriteString("  </state>\n")
	b.WriteString("  <progress>\n")
	b.WriteString(fmt.Sprintf("    <time_remaining_s>%d</time_remaining_s>\n", data.TimeRemainingS))
	b.WriteString(fmt.Sprintf("    <questions_remaining>%d</questions_remaining>\n", data.QuestionsRemaining))
	b.WriteString(fmt.Sprintf("    <criteria_remaining>%d</criteria_remaining>\n", data.CriteriaRemaining))
	b.WriteString("    <required_language>\n")
	b.WriteString("      <label>" + xmlEscape(data.RequiredLanguageLabel) + "</label>\n")
	b.WriteString("      <code>" + xmlEscape(data.RequiredLanguageCode) + "</code>\n")
	b.WriteString("    </required_language>\n")
	b.WriteString("  </progress>\n")
	b.WriteString("  <criteria_coverage>\n")
	for _, area := range data.CriteriaCoverage {
		b.WriteString("    <area>\n")
		b.WriteString(fmt.Sprintf("      <id>%d</id>\n", area.ID))
		b.WriteString("      <name>" + xmlEscape(area.Name) + "</name>\n")
		b.WriteString("      <status>" + xmlEscape(area.Status) + "</status>\n")
		b.WriteString("    </area>\n")
	}
	b.WriteString("  </criteria_coverage>\n")
	if data.Priority != "" {
		b.WriteString("  <priority>" + xmlEscape(data.Priority) + "</priority>\n")
	}
	if data.CurrentQuestionText != "" {
		b.WriteString("  <current_question>" + xmlEscape(data.CurrentQuestionText) + "</current_question>\n")
	}
	if data.LatestAnswerText != "" {
		b.WriteString("  <candidate_answer>" + xmlEscape(data.LatestAnswerText) + "</candidate_answer>\n")
	}
	b.WriteString("  <instruction>" + xmlEscape(data.Instruction) + "</instruction>\n")
	b.WriteString("</turn>")
	return b.String()
}

func requiredLanguageInfo(preferredLanguage string) (code, label string) {
	if strings.EqualFold(strings.TrimSpace(preferredLanguage), "en") {
		return "en", "English"
	}
	return "es", "Spanish"
}

func newClaudeTextMessage(role, text string) map[string]interface{} {
	return map[string]interface{}{
		"role": role,
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": text,
			},
		},
	}
}

func addCacheControlToClaudeMessage(message map[string]interface{}) {
	content, ok := message["content"].([]map[string]interface{})
	if !ok || len(content) == 0 {
		return
	}
	content[len(content)-1]["cache_control"] = map[string]string{"type": "ephemeral"}
}

func xmlEscape(value string) string {
	var b bytes.Buffer
	if err := xml.EscapeText(&b, []byte(value)); err != nil {
		return value
	}
	return b.String()
}

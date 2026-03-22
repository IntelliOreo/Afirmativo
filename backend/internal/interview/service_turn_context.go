package interview

import (
	"log/slog"
	"strings"

	"github.com/afirmativo/backend/internal/config"
)

func (s *Service) orderedAreaSlugs() []string {
	slugs := make([]string, 0, len(s.settings.AreaConfigs))
	for _, cfg := range s.settings.AreaConfigs {
		slugs = append(slugs, cfg.Slug)
	}
	return slugs
}

func (s *Service) findAreaConfig(slug string) (config.AreaConfig, int) {
	for i, ac := range s.settings.AreaConfigs {
		if ac.Slug == slug {
			return ac, i
		}
	}
	// Return a minimal config if not found (shouldn't happen in practice).
	return config.AreaConfig{Slug: slug, Label: slug}, -1
}

func (s *Service) buildCriteriaCoverage(areas []QuestionArea) []CriteriaCoverage {
	coverage := make([]CriteriaCoverage, 0, len(areas))
	for _, a := range areas {
		cfg, _ := s.findAreaConfig(a.Area)
		coverage = append(coverage, CriteriaCoverage{
			ID:     cfg.ID,
			Name:   a.Area,
			Status: a.Status,
		})
	}
	return coverage
}

func (s *Service) buildHistoryTurns(answers []Answer, preferredLanguage string) []HistoryTurn {
	useEnglish := strings.EqualFold(strings.TrimSpace(preferredLanguage), "en")

	historyTurns := make([]HistoryTurn, 0, len(answers))
	for _, a := range answers {
		historyTurns = append(historyTurns, HistoryTurn{
			QuestionText: a.QuestionText,
			AnswerText:   selectTranscript(useEnglish, a.TranscriptEn, a.TranscriptEs),
		})
	}
	return historyTurns
}

func (s *Service) countCriteriaRemaining(areas []QuestionArea) int {
	count := 0
	for _, a := range areas {
		if isAreaUnresolved(a.Status) {
			count++
		}
	}
	return count
}

func (s *Service) buildAITurnContext(
	area QuestionArea,
	areaCfg config.AreaConfig,
	areaIndex int,
	answers []Answer,
	areas []QuestionArea,
	preferredLanguage string,
	totalBudgetS int,
	timeRemainingS int,
	isOpening bool,
	questionText string,
	answerText string,
) *AITurnContext {
	return &AITurnContext{
		PreferredLanguage:   preferredLanguage,
		CurrentAreaSlug:     area.Area,
		CurrentAreaID:       areaCfg.ID,
		CurrentAreaIndex:    areaIndex,
		IsOpeningTurn:       isOpening,
		CurrentQuestionText: questionText,
		LatestAnswerText:    answerText,
		CurrentAreaLabel:    areaCfg.Label,
		Description:         areaCfg.Description,
		SufficiencyReqs:     areaCfg.SufficiencyRequirements,
		AreaStatus:          area.Status,
		IsPreAddressed:      area.Status == AreaStatusPreAddressed,
		FollowUpsRemaining:  MaxQuestionsPerArea - area.QuestionsCount,
		TotalBudgetS:        totalBudgetS,
		TimeRemainingS:      timeRemainingS,
		QuestionsRemaining:  EstimatedTotalQuestions - len(answers),
		CriteriaRemaining:   s.countCriteriaRemaining(areas),
		CriteriaCoverage:    s.buildCriteriaCoverage(areas),
		HistoryTurns:        s.buildHistoryTurns(answers, preferredLanguage),
	}
}

// matchAreaSlug tries to find a matching area slug from the AI's cross-criteria name.
// Uses case-insensitive matching against both slugs and labels.
func (s *Service) matchAreaSlug(name string) string {
	lower := strings.ToLower(name)
	for _, ac := range s.settings.AreaConfigs {
		if strings.ToLower(ac.Slug) == lower || strings.ToLower(ac.Label) == lower {
			return ac.Slug
		}
	}
	return ""
}

func selectTranscript(preferEnglish bool, en, es string) string {
	if preferEnglish {
		if candidate := strings.TrimSpace(en); candidate != "" {
			return en
		}
		return es
	}
	if candidate := strings.TrimSpace(es); candidate != "" {
		return es
	}
	return en
}

func (s *Service) extractPreAddressed(other []OtherCriterion) []PreAddressedArea {
	flags := make([]PreAddressedArea, 0, len(other))
	for _, item := range other {
		slug := s.matchAreaSlug(item.Name)
		if slug == "" {
			slog.Warn("cross-criteria flag: no matching area", "name", item.Name)
			continue
		}
		flags = append(flags, PreAddressedArea{
			Slug:     slug,
			Evidence: item.EvidenceSummary,
		})
	}
	return flags
}

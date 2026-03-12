package interview

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestPostgresStoreAdvanceNonCriterionStepRecordsEventAndAdvancesFlow(t *testing.T) {
	store, cleanup := newPostgresIntegrationStore(t)
	defer cleanup()

	ctx := context.Background()
	sessionCode := "AP-FLOW-DISCLAIMER"
	insertPostgresIntegrationSession(t, store.pool, postgresIntegrationSessionParams{
		SessionCode:           sessionCode,
		Status:                "created",
		FlowStep:              FlowStepDisclaimer,
		ExpectedTurnID:        "turn-disclaimer",
		DisplayQuestionNumber: 1,
	})

	got, err := store.AdvanceNonCriterionStep(ctx, AdvanceNonCriterionStepParams{
		SessionCode:    sessionCode,
		ExpectedTurnID: "turn-disclaimer",
		CurrentStep:    FlowStepDisclaimer,
		NextStep:       FlowStepReadiness,
		EventType:      "disclaimer_ack",
		AnswerText:     "Yes, I understand.",
		NextIssuedQuestion: NewIssuedQuestion(&Question{
			TextEs:         "Listo para continuar",
			TextEn:         "Ready to continue",
			Area:           "history",
			Kind:           QuestionKindReadiness,
			TurnID:         "turn-readiness",
			QuestionNumber: 1,
			TotalQuestions: EstimatedTotalQuestions,
		}, time.Now().UTC(), 300),
	})
	if err != nil {
		t.Fatalf("AdvanceNonCriterionStep() error = %v", err)
	}
	if got.Step != FlowStepReadiness {
		t.Fatalf("got.Step = %q, want %q", got.Step, FlowStepReadiness)
	}
	if got.ExpectedTurnID != "turn-readiness" {
		t.Fatalf("got.ExpectedTurnID = %q, want turn-readiness", got.ExpectedTurnID)
	}
	if got.QuestionNumber != 1 {
		t.Fatalf("got.QuestionNumber = %d, want 1", got.QuestionNumber)
	}
	if got.ActiveQuestion == nil || got.ActiveQuestion.Question.TurnID != "turn-readiness" {
		t.Fatalf("got.ActiveQuestion = %#v, want readiness turn", got.ActiveQuestion)
	}

	flowState, err := store.GetFlowState(ctx, sessionCode)
	if err != nil {
		t.Fatalf("GetFlowState() error = %v", err)
	}
	if flowState.Step != FlowStepReadiness {
		t.Fatalf("flowState.Step = %q, want %q", flowState.Step, FlowStepReadiness)
	}
	if flowState.ExpectedTurnID != "turn-readiness" {
		t.Fatalf("flowState.ExpectedTurnID = %q, want turn-readiness", flowState.ExpectedTurnID)
	}
	if flowState.ActiveQuestion == nil || flowState.ActiveQuestion.Question.Kind != QuestionKindReadiness {
		t.Fatalf("flowState.ActiveQuestion = %#v, want persisted readiness question", flowState.ActiveQuestion)
	}

	events := loadPostgresIntegrationEvents(t, store.pool, sessionCode)
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].EventType != "disclaimer_ack" {
		t.Fatalf("events[0].EventType = %q, want disclaimer_ack", events[0].EventType)
	}
	if events[0].AnswerText != "Yes, I understand." {
		t.Fatalf("events[0].AnswerText = %q, want recorded answer", events[0].AnswerText)
	}
}

func TestPostgresStoreAdvanceNonCriterionStepConflictRollsBack(t *testing.T) {
	store, cleanup := newPostgresIntegrationStore(t)
	defer cleanup()

	ctx := context.Background()
	sessionCode := "AP-FLOW-CONFLICT"
	insertPostgresIntegrationSession(t, store.pool, postgresIntegrationSessionParams{
		SessionCode:           sessionCode,
		Status:                "created",
		FlowStep:              FlowStepDisclaimer,
		ExpectedTurnID:        "turn-disclaimer",
		DisplayQuestionNumber: 1,
	})

	_, err := store.AdvanceNonCriterionStep(ctx, AdvanceNonCriterionStepParams{
		SessionCode:    sessionCode,
		ExpectedTurnID: "wrong-turn",
		CurrentStep:    FlowStepDisclaimer,
		NextStep:       FlowStepReadiness,
		EventType:      "disclaimer_ack",
		AnswerText:     "This should not persist.",
		NextIssuedQuestion: NewIssuedQuestion(&Question{
			TextEs:         "Listo para continuar",
			TextEn:         "Ready to continue",
			Area:           "history",
			Kind:           QuestionKindReadiness,
			TurnID:         "turn-readiness",
			QuestionNumber: 1,
			TotalQuestions: EstimatedTotalQuestions,
		}, time.Now().UTC(), 300),
	})
	assertPostgresIntegrationConflict(t, err)

	flowState, err := store.GetFlowState(ctx, sessionCode)
	if err != nil {
		t.Fatalf("GetFlowState() error = %v", err)
	}
	if flowState.Step != FlowStepDisclaimer {
		t.Fatalf("flowState.Step = %q, want %q", flowState.Step, FlowStepDisclaimer)
	}
	if flowState.ExpectedTurnID != "turn-disclaimer" {
		t.Fatalf("flowState.ExpectedTurnID = %q, want turn-disclaimer", flowState.ExpectedTurnID)
	}

	events := loadPostgresIntegrationEvents(t, store.pool, sessionCode)
	if len(events) != 0 {
		t.Fatalf("len(events) = %d, want 0", len(events))
	}
}

func TestPostgresStoreProcessCriterionTurnMovesToNextArea(t *testing.T) {
	store, cleanup := newPostgresIntegrationStore(t)
	defer cleanup()

	ctx := context.Background()
	sessionCode := "AP-CRITERION-NEXT"
	insertPostgresIntegrationSession(t, store.pool, postgresIntegrationSessionParams{
		SessionCode:           sessionCode,
		Status:                "interviewing",
		FlowStep:              FlowStepCriterion,
		ExpectedTurnID:        "turn-current",
		DisplayQuestionNumber: 3,
	})
	insertPostgresIntegrationArea(t, store.pool, postgresIntegrationAreaParams{
		SessionCode:    sessionCode,
		Area:           "history",
		Status:         AreaStatusInProgress,
		QuestionsCount: 0,
	})
	insertPostgresIntegrationArea(t, store.pool, postgresIntegrationAreaParams{
		SessionCode: sessionCode,
		Area:        "nexus",
		Status:      AreaStatusPending,
	})
	insertPostgresIntegrationArea(t, store.pool, postgresIntegrationAreaParams{
		SessionCode: sessionCode,
		Area:        "harm",
		Status:      AreaStatusPending,
	})

	evaluation := &Evaluation{
		CurrentCriterion: CurrentCriterion{
			ID:              1,
			Status:          "sufficient",
			EvidenceSummary: "Detailed answer",
			Recommendation:  "move_on",
		},
	}

	got, err := store.ProcessCriterionTurn(ctx, ProcessCriterionTurnParams{
		SessionCode:       sessionCode,
		ExpectedTurnID:    "turn-current",
		CurrentArea:       "history",
		QuestionText:      "What happened to you?",
		AnswerText:        "I was targeted because of my opinion.",
		PreferredLanguage: "en",
		Evaluation:        evaluation,
		PreAddressed: []PreAddressedArea{
			{Slug: "nexus", Evidence: "Opinion-based harm already described."},
		},
		Decision: CriterionTurnDecision{
			Action:        CriterionTurnActionNext,
			MarkCurrentAs: AreaStatusComplete,
		},
		NextArea: "nexus",
		NextIssuedQuestion: NewIssuedQuestion(&Question{
			TextEs:         "Why does the harm connect to a protected ground?",
			TextEn:         "Why does the harm connect to a protected ground?",
			Area:           "nexus",
			Kind:           QuestionKindCriterion,
			TurnID:         "turn-next",
			QuestionNumber: 4,
			TotalQuestions: EstimatedTotalQuestions,
		}, time.Now().UTC(), 300),
	})
	if err != nil {
		t.Fatalf("ProcessCriterionTurn() error = %v", err)
	}
	if got.NewCount != 1 {
		t.Fatalf("got.NewCount = %d, want 1", got.NewCount)
	}

	answers, err := store.GetAnswersBySession(ctx, sessionCode)
	if err != nil {
		t.Fatalf("GetAnswersBySession() error = %v", err)
	}
	if len(answers) != 1 {
		t.Fatalf("len(answers) = %d, want 1", len(answers))
	}
	if answers[0].Area != "history" {
		t.Fatalf("answers[0].Area = %q, want history", answers[0].Area)
	}
	if answers[0].TranscriptEn != "I was targeted because of my opinion." {
		t.Fatalf("answers[0].TranscriptEn = %q, want saved English transcript", answers[0].TranscriptEn)
	}
	if answers[0].TranscriptEs != "" {
		t.Fatalf("answers[0].TranscriptEs = %q, want empty", answers[0].TranscriptEs)
	}
	if answers[0].Sufficiency != "sufficient" {
		t.Fatalf("answers[0].Sufficiency = %q, want sufficient", answers[0].Sufficiency)
	}

	wantEvalJSON, err := json.Marshal(evaluation)
	if err != nil {
		t.Fatalf("json.Marshal(evaluation) error = %v", err)
	}
	mustEqualPostgresIntegrationJSON(t, []byte(answers[0].AIEvaluationJSON), wantEvalJSON)

	areas, err := store.GetAreasBySession(ctx, sessionCode)
	if err != nil {
		t.Fatalf("GetAreasBySession() error = %v", err)
	}
	history := mustGetPostgresIntegrationArea(t, areas, "history")
	if history.Status != AreaStatusComplete {
		t.Fatalf("history.Status = %q, want %q", history.Status, AreaStatusComplete)
	}
	if history.QuestionsCount != 1 {
		t.Fatalf("history.QuestionsCount = %d, want 1", history.QuestionsCount)
	}
	if mustGetPostgresIntegrationEndedAt(t, store.pool, sessionCode, "history") == nil {
		t.Fatal("history area_ended_at = nil, want non-nil")
	}

	nexus := mustGetPostgresIntegrationArea(t, areas, "nexus")
	if nexus.Status != AreaStatusInProgress {
		t.Fatalf("nexus.Status = %q, want %q", nexus.Status, AreaStatusInProgress)
	}
	if nexus.PreAddressedEvidence != "Opinion-based harm already described." {
		t.Fatalf("nexus.PreAddressedEvidence = %q, want saved evidence", nexus.PreAddressedEvidence)
	}

	harm := mustGetPostgresIntegrationArea(t, areas, "harm")
	if harm.Status != AreaStatusPending {
		t.Fatalf("harm.Status = %q, want %q", harm.Status, AreaStatusPending)
	}

	flowState, err := store.GetFlowState(ctx, sessionCode)
	if err != nil {
		t.Fatalf("GetFlowState() error = %v", err)
	}
	if flowState.Step != FlowStepCriterion {
		t.Fatalf("flowState.Step = %q, want %q", flowState.Step, FlowStepCriterion)
	}
	if flowState.ExpectedTurnID != "turn-next" {
		t.Fatalf("flowState.ExpectedTurnID = %q, want turn-next", flowState.ExpectedTurnID)
	}
	if flowState.QuestionNumber != 4 {
		t.Fatalf("flowState.QuestionNumber = %d, want 4", flowState.QuestionNumber)
	}
	if flowState.ActiveQuestion == nil || flowState.ActiveQuestion.Question.Area != "nexus" {
		t.Fatalf("flowState.ActiveQuestion = %#v, want persisted next-area question", flowState.ActiveQuestion)
	}
}

func TestPostgresStoreProcessCriterionTurnMarksDoneWhenNoAreasRemain(t *testing.T) {
	store, cleanup := newPostgresIntegrationStore(t)
	defer cleanup()

	ctx := context.Background()
	sessionCode := "AP-CRITERION-DONE"
	insertPostgresIntegrationSession(t, store.pool, postgresIntegrationSessionParams{
		SessionCode:           sessionCode,
		Status:                "interviewing",
		FlowStep:              FlowStepCriterion,
		ExpectedTurnID:        "turn-final",
		DisplayQuestionNumber: 5,
	})
	insertPostgresIntegrationArea(t, store.pool, postgresIntegrationAreaParams{
		SessionCode:    sessionCode,
		Area:           "history",
		Status:         AreaStatusInProgress,
		QuestionsCount: 0,
	})

	evaluation := &Evaluation{
		CurrentCriterion: CurrentCriterion{
			ID:              1,
			Status:          "insufficient",
			EvidenceSummary: "Not enough detail",
			Recommendation:  "move_on",
		},
	}

	got, err := store.ProcessCriterionTurn(ctx, ProcessCriterionTurnParams{
		SessionCode:       sessionCode,
		ExpectedTurnID:    "turn-final",
		CurrentArea:       "history",
		QuestionText:      "Why are you seeking protection?",
		AnswerText:        "Porque tengo miedo de regresar.",
		PreferredLanguage: "es",
		Evaluation:        evaluation,
		Decision: CriterionTurnDecision{
			Action:        CriterionTurnActionNext,
			MarkCurrentAs: AreaStatusInsufficient,
		},
	})
	if err != nil {
		t.Fatalf("ProcessCriterionTurn() error = %v", err)
	}
	if got.NewCount != 1 {
		t.Fatalf("got.NewCount = %d, want 1", got.NewCount)
	}

	answers, err := store.GetAnswersBySession(ctx, sessionCode)
	if err != nil {
		t.Fatalf("GetAnswersBySession() error = %v", err)
	}
	if len(answers) != 1 {
		t.Fatalf("len(answers) = %d, want 1", len(answers))
	}
	if answers[0].TranscriptEs != "Porque tengo miedo de regresar." {
		t.Fatalf("answers[0].TranscriptEs = %q, want saved Spanish transcript", answers[0].TranscriptEs)
	}
	if answers[0].TranscriptEn != "" {
		t.Fatalf("answers[0].TranscriptEn = %q, want empty", answers[0].TranscriptEn)
	}
	if answers[0].Sufficiency != "insufficient" {
		t.Fatalf("answers[0].Sufficiency = %q, want insufficient", answers[0].Sufficiency)
	}

	areas, err := store.GetAreasBySession(ctx, sessionCode)
	if err != nil {
		t.Fatalf("GetAreasBySession() error = %v", err)
	}
	history := mustGetPostgresIntegrationArea(t, areas, "history")
	if history.Status != AreaStatusInsufficient {
		t.Fatalf("history.Status = %q, want %q", history.Status, AreaStatusInsufficient)
	}
	if history.QuestionsCount != 1 {
		t.Fatalf("history.QuestionsCount = %d, want 1", history.QuestionsCount)
	}
	if mustGetPostgresIntegrationEndedAt(t, store.pool, sessionCode, "history") == nil {
		t.Fatal("history area_ended_at = nil, want non-nil")
	}

	flowState, err := store.GetFlowState(ctx, sessionCode)
	if err != nil {
		t.Fatalf("GetFlowState() error = %v", err)
	}
	if flowState.Step != FlowStepDone {
		t.Fatalf("flowState.Step = %q, want %q", flowState.Step, FlowStepDone)
	}
	if flowState.ExpectedTurnID != "" {
		t.Fatalf("flowState.ExpectedTurnID = %q, want empty", flowState.ExpectedTurnID)
	}
	if flowState.QuestionNumber != 6 {
		t.Fatalf("flowState.QuestionNumber = %d, want 6", flowState.QuestionNumber)
	}
	if flowState.ActiveQuestion != nil {
		t.Fatalf("flowState.ActiveQuestion = %#v, want nil after flow completion", flowState.ActiveQuestion)
	}
}

func TestPostgresStoreProcessCriterionTurnConflictRollsBack(t *testing.T) {
	store, cleanup := newPostgresIntegrationStore(t)
	defer cleanup()

	ctx := context.Background()
	sessionCode := "AP-CRITERION-CONFLICT"
	insertPostgresIntegrationSession(t, store.pool, postgresIntegrationSessionParams{
		SessionCode:           sessionCode,
		Status:                "interviewing",
		FlowStep:              FlowStepCriterion,
		ExpectedTurnID:        "turn-current",
		DisplayQuestionNumber: 2,
	})
	insertPostgresIntegrationArea(t, store.pool, postgresIntegrationAreaParams{
		SessionCode:    sessionCode,
		Area:           "history",
		Status:         AreaStatusInProgress,
		QuestionsCount: 0,
	})
	insertPostgresIntegrationArea(t, store.pool, postgresIntegrationAreaParams{
		SessionCode: sessionCode,
		Area:        "harm",
		Status:      AreaStatusPending,
	})

	_, err := store.ProcessCriterionTurn(ctx, ProcessCriterionTurnParams{
		SessionCode:       sessionCode,
		ExpectedTurnID:    "wrong-turn",
		CurrentArea:       "history",
		QuestionText:      "What happened?",
		AnswerText:        "Nothing should save.",
		PreferredLanguage: "en",
		Evaluation:        &Evaluation{CurrentCriterion: CurrentCriterion{Status: "sufficient"}},
		Decision: CriterionTurnDecision{
			Action:        CriterionTurnActionNext,
			MarkCurrentAs: AreaStatusComplete,
		},
		NextArea: "harm",
		NextIssuedQuestion: NewIssuedQuestion(&Question{
			TextEs:         "What harm do you fear?",
			TextEn:         "What harm do you fear?",
			Area:           "harm",
			Kind:           QuestionKindCriterion,
			TurnID:         "turn-next",
			QuestionNumber: 3,
			TotalQuestions: EstimatedTotalQuestions,
		}, time.Now().UTC(), 300),
	})
	assertPostgresIntegrationConflict(t, err)

	answerCount, err := store.GetAnswerCount(ctx, sessionCode)
	if err != nil {
		t.Fatalf("GetAnswerCount() error = %v", err)
	}
	if answerCount != 0 {
		t.Fatalf("answerCount = %d, want 0", answerCount)
	}

	areas, err := store.GetAreasBySession(ctx, sessionCode)
	if err != nil {
		t.Fatalf("GetAreasBySession() error = %v", err)
	}
	history := mustGetPostgresIntegrationArea(t, areas, "history")
	if history.Status != AreaStatusInProgress {
		t.Fatalf("history.Status = %q, want %q", history.Status, AreaStatusInProgress)
	}
	if history.QuestionsCount != 0 {
		t.Fatalf("history.QuestionsCount = %d, want 0", history.QuestionsCount)
	}

	harm := mustGetPostgresIntegrationArea(t, areas, "harm")
	if harm.Status != AreaStatusPending {
		t.Fatalf("harm.Status = %q, want %q", harm.Status, AreaStatusPending)
	}

	flowState, err := store.GetFlowState(ctx, sessionCode)
	if err != nil {
		t.Fatalf("GetFlowState() error = %v", err)
	}
	if flowState.Step != FlowStepCriterion {
		t.Fatalf("flowState.Step = %q, want %q", flowState.Step, FlowStepCriterion)
	}
	if flowState.ExpectedTurnID != "turn-current" {
		t.Fatalf("flowState.ExpectedTurnID = %q, want turn-current", flowState.ExpectedTurnID)
	}
	if flowState.QuestionNumber != 2 {
		t.Fatalf("flowState.QuestionNumber = %d, want 2", flowState.QuestionNumber)
	}
}

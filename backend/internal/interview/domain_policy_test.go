package interview

import "testing"

func TestDecideCriterionTurn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		current           CurrentCriterion
		questionsCount    int
		maxQuestions      int
		wantAction        CriterionTurnAction
		wantMarkCurrentAs AreaStatus
	}{
		{
			name: "sufficient_moves_next_and_completes",
			current: CurrentCriterion{
				Status:         "sufficient",
				Recommendation: "follow_up",
			},
			questionsCount:    1,
			maxQuestions:      MaxQuestionsPerArea,
			wantAction:        CriterionTurnActionNext,
			wantMarkCurrentAs: AreaStatusComplete,
		},
		{
			name: "move_on_marks_insufficient",
			current: CurrentCriterion{
				Status:         "partially_sufficient",
				Recommendation: "move_on",
			},
			questionsCount:    2,
			maxQuestions:      MaxQuestionsPerArea,
			wantAction:        CriterionTurnActionNext,
			wantMarkCurrentAs: AreaStatusInsufficient,
		},
		{
			name: "max_question_budget_marks_insufficient",
			current: CurrentCriterion{
				Status:         "insufficient",
				Recommendation: "follow_up",
			},
			questionsCount:    6,
			maxQuestions:      6,
			wantAction:        CriterionTurnActionNext,
			wantMarkCurrentAs: AreaStatusInsufficient,
		},
		{
			name: "otherwise_stays_in_current_area",
			current: CurrentCriterion{
				Status:         "partially_sufficient",
				Recommendation: "follow_up",
			},
			questionsCount:    3,
			maxQuestions:      6,
			wantAction:        CriterionTurnActionStay,
			wantMarkCurrentAs: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := DecideCriterionTurn(tc.current, tc.questionsCount, tc.maxQuestions)
			if got.Action != tc.wantAction {
				t.Fatalf("action = %q, want %q", got.Action, tc.wantAction)
			}
			if got.MarkCurrentAs != tc.wantMarkCurrentAs {
				t.Fatalf("markCurrentAs = %q, want %q", got.MarkCurrentAs, tc.wantMarkCurrentAs)
			}
		})
	}
}

func TestSelectNextPendingArea(t *testing.T) {
	t.Parallel()

	ordered := []string{"area-a", "area-b", "area-c"}
	statusByArea := map[string]AreaStatus{
		"area-a": AreaStatusComplete,
		"area-b": AreaStatusPreAddressed,
		"area-c": AreaStatusPending,
	}

	got := SelectNextPendingArea(ordered, statusByArea)
	if got != "area-b" {
		t.Fatalf("next area = %q, want area-b", got)
	}

	none := SelectNextPendingArea(
		ordered,
		map[string]AreaStatus{
			"area-a": AreaStatusComplete,
			"area-b": AreaStatusInsufficient,
			"area-c": AreaStatusNotAssessed,
		},
	)
	if none != "" {
		t.Fatalf("next area = %q, want empty", none)
	}
}

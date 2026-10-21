package modelroute

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestModelCatalogIncludesFableSolAndIndependentEfforts(t *testing.T) {
	catalog := DefaultCandidates()
	if len(catalog) != 7 {
		t.Fatalf("catalog=%d", len(catalog))
	}
	models := map[string]Candidate{}
	for _, c := range catalog {
		if c.Effort != "" || len(c.Efforts) < 5 || c.Description == "" {
			t.Fatalf("pinned or undescribed candidate=%+v", c)
		}
		models[c.Model] = c
		for _, e := range []string{"low", "medium", "high", "xhigh", "max"} {
			if !SupportsEffort(c, e) {
				t.Fatalf("missing effort %s/%s", c.Model, e)
			}
		}
		if c.Agent == "claude" && SupportsEffort(c, "ultra") {
			t.Fatal("Claude ultra")
		}
	}
	if models["claude-fable-5-1"].Model == "" || models["gpt-6.1-sol"].Model == "" {
		t.Fatal("omitted Fable/Sol")
	}
	if SupportsEffort(models["gpt-6-luna"], "ultra") || !SupportsEffort(models["gpt-6.1-sol"], "ultra") {
		t.Fatal("wrong Codex CLI effort capabilities")
	}
	catalog[0].Efforts[0] = "bad"
	if DefaultCandidates()[0].Efforts[0] != "low" {
		t.Fatal("mutated shared catalog")
	}
}

func TestEffortQuestionsArePerModelInOneBatch(t *testing.T) {
	in := Input{Candidates: DefaultCandidates()}
	q, profiles := decisionQuestions(in)
	if len(q) != 9 || len(profiles) != 7 {
		t.Fatalf("questions=%d models=%d", len(q), len(profiles))
	}
	for key, c := range profiles {
		effort := q["effort_"+key].(map[string]any)
		criteria := effort["criteria"].(map[string]string)
		if len(criteria) != len(c.Efforts) || !strings.Contains(effort["instructions"].(string), c.Model) {
			t.Fatalf("effort question %s=%v", c.Model, effort)
		}
	}
	now := time.Now()
	pinned, err := Prepare("task", "codex", "", "high", quotaSnapshot(now, 1, 2, 3), now)
	if err != nil {
		t.Fatal(err)
	}
	q, _ = decisionQuestions(pinned)
	if len(q) != 2 {
		t.Fatalf("explicit effort generated effort questions: %v", q)
	}
	for _, c := range pinned.Candidates {
		if c.Effort != "high" || len(c.Efforts) != 0 {
			t.Fatal(c)
		}
	}
}

func TestSelectionUsesSelectedModelsEffort(t *testing.T) {
	for _, tt := range []struct{ model, effort string }{{"claude-fable-5-1", "medium"}, {"claude-haiku-5-5", "max"}, {"gpt-6.1-sol", "high"}, {"gpt-6-astra", "low"}} {
		t.Run(tt.model+"/"+tt.effort, func(t *testing.T) {
			var candidate Candidate
			for _, c := range DefaultCandidates() {
				if c.Model == tt.model {
					candidate = c
				}
			}
			raw, _ := json.Marshal(map[string]any{"answers": map[string]any{"route": map[string]string{"choice": "profile_0"}, "effort_profile_0": map[string]string{"choice": tt.effort}, "effort_profile_1": map[string]string{"choice": "not-an-effort"}}, "confidences": map[string]float64{"route": .1, "effort_profile_0": .2}})
			s, err := ParseSelection(Input{}, map[string]Candidate{"profile_0": candidate}, raw)
			if err != nil {
				t.Fatal(err)
			}
			if s.Model != tt.model || s.Effort != tt.effort || s.EffortConfidence != .2 {
				t.Fatalf("selection=%+v", s)
			}
		})
	}
	for _, raw := range []string{`{"answers":{"route":{"choice":"profile_0"}}}`, `{"answers":{"route":{"choice":"profile_0"},"effort_profile_0":{"choice":"ultra"}}}`} {
		_, err := ParseSelection(Input{}, map[string]Candidate{"profile_0": DefaultCandidates()[0]}, []byte(raw))
		if err == nil {
			t.Fatal("accepted missing/unsupported effort")
		}
	}
}

func TestChooseModelAndEffortUsesExactlyOneDecideInvocation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMACT_ROUTING_LOG", "off")
	t.Setenv("DECIDE_CLI_CONFIG", "none")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("EFFORT_TEST_CALLS", filepath.Join(dir, "calls"))
	t.Setenv("EFFORT_TEST_QUESTIONS", filepath.Join(dir, "questions.json"))
	script := `#!/bin/sh
printf 'call\n' >> "$EFFORT_TEST_CALLS"
while [ "$#" -gt 0 ]; do
 if [ "$1" = "--questions" ]; then shift; cp "$1" "$EFFORT_TEST_QUESTIONS"; fi
 shift
done
cat >/dev/null
printf '%s\n' '{"answers":{"route":{"choice":"profile_0"},"effort_profile_0":{"choice":"xhigh"}},"confidences":{"route":0.1,"effort_profile_0":0.0}}'
`
	os.WriteFile(filepath.Join(dir, "decide"), []byte(script), 0700)
	var sol Candidate
	for _, c := range DefaultCandidates() {
		if c.Model == "gpt-6.1-sol" {
			sol = c
		}
	}
	s, err := Choose(context.Background(), Input{Candidates: []Candidate{sol}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Model != "gpt-6.1-sol" || s.Effort != "xhigh" {
		t.Fatal(s)
	}
	calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if string(calls) != "call\n" {
		t.Fatalf("calls=%s", calls)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "questions.json"))
	var questions map[string]any
	json.Unmarshal(raw, &questions)
	if len(questions) != 3 || questions["effort_profile_0"] == nil {
		t.Fatalf("questions=%s", raw)
	}
}

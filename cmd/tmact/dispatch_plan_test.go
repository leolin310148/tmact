package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leolin310148/tmact/internal/askreply"
	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/modelroute"
)

func TestDispatchPlanApplyReusesDecision(t *testing.T) {
	defer stubCLIHooks(t)()
	dir := t.TempDir()
	file := filepath.Join(dir, "task.md")
	os.WriteFile(file, []byte("implement task"), 0600)
	calls := 0
	dispatchRun = func(o dispatch.Options) (dispatch.Report, error) {
		calls++
		if calls == 1 {
			if !o.AutoModel || o.Execute {
				t.Fatalf("plan opts: %#v", o)
			}
		} else {
			if o.AutoModel || !o.Execute || o.Model != "gpt-6.1-sol" || o.Effort != "medium" || o.Selection == nil || o.PlanState == nil || o.PlanFiles == nil {
				t.Fatalf("apply opts: %#v", o)
			}
		}
		return dispatch.Report{Session: o.Session, Dir: o.Dir, Prompt: o.Prompt, Agent: "codex", Model: "gpt-6.1-sol", Effort: "medium", DispatchID: "plan-test", Selection: &modelroute.Selection{Status: "selected", Agent: "codex", Model: "gpt-6.1-sol", Effort: "medium", Files: planTestFileMetadata(o.Prompt, o.Dir)}, PlanState: &dispatch.PlanState{}, Execute: o.Execute}, nil
	}
	out, err := captureRun(t, "dispatch-work", "plan-work", "--dir", dir, "--auto-model", "--prompt", "Read "+file, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report dispatch.Report
	if err = json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.PlanFile == "" {
		t.Fatal("no automatic plan")
	}
	defer os.RemoveAll(filepath.Dir(report.PlanFile))
	info, _ := os.Stat(report.PlanFile)
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	_, err = captureRun(t, "dispatch-work", "--apply-plan", report.PlanFile, "--execute", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("dispatch calls=%d", calls)
	}
	os.WriteFile(file, []byte("changed task"), 0600)
	_, err = captureRun(t, "dispatch-work", "--apply-plan", report.PlanFile, "--execute")
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("changed file: %v", err)
	}
	if calls != 2 {
		t.Fatal("stale plan reached dispatch")
	}
}

func TestPlanRejectsOverridesAndEdits(t *testing.T) {
	dir := t.TempDir()
	r := dispatch.Report{Session: "work", Dir: dir, Agent: "codex", Prompt: "go", PlanState: &dispatch.PlanState{}}
	path, err := writeDispatchPlan("dispatch-work", []string{"work", "--dir", dir, "--agent", "codex", "--prompt", "go"}, r)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(filepath.Dir(path))
	for _, args := range [][]string{{"--apply-plan", path, "--prompt", "other"}, {"--apply-plan", path, "--model", "gpt-5.4"}} {
		if _, _, err = expandApplyPlan("dispatch-work", args); err == nil {
			t.Fatal("accepted override")
		}
	}
	if _, _, err = expandApplyPlan("ask", []string{"--apply-plan", path}); err == nil {
		t.Fatal("accepted wrong command")
	}
	b, _ := os.ReadFile(path)
	b = []byte(strings.Replace(string(b), `"go"`, `"changed"`, 1))
	os.WriteFile(path, b, 0600)
	if _, _, err = expandApplyPlan("dispatch-work", []string{"--apply-plan", path}); err == nil {
		t.Fatal("accepted edited plan")
	}
}

func TestPlanFindsChangesBeyondRoutingExcerpt(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "task.md")
	content := strings.Repeat("x", modelroute.MaxTaskFileBytes+10)
	os.WriteFile(file, []byte(content), 0600)
	r := dispatch.Report{Dir: dir, Agent: "codex", Prompt: file, PlanState: &dispatch.PlanState{}}
	path, err := writeDispatchPlan("ask", []string{"work", "--dir", dir, "--agent", "codex", "--prompt", file}, r)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(filepath.Dir(path))
	os.WriteFile(file, []byte(content[:len(content)-1]+"y"), 0600)
	if _, _, err = expandApplyPlan("ask", []string{"--apply-plan", path}); err == nil {
		t.Fatal("missed change beyond excerpt")
	}
}

func TestPlanFlagLikePromptIsData(t *testing.T) {
	args := []string{"work", "--prompt", "--apply-plan", "--dir", "/tmp", "--agent", "codex"}
	got, p, err := expandApplyPlan("ask", args)
	if err != nil || p != nil || len(got) != len(args) {
		t.Fatalf("prompt treated as flag: %v", err)
	}
}

func TestAskAutomaticPlanApplyUsesOneDecisionAndNewCapability(t *testing.T) {
	defer stubCLIHooks(t)()
	dir, storeDir := t.TempDir(), t.TempDir()
	decisions := 0
	executions := 0
	dispatchRun = func(o dispatch.Options) (dispatch.Report, error) {
		if o.AutoModel {
			decisions++
		}
		if o.Execute {
			executions++
			if o.AutoModel || o.Model != "gpt-6.1-sol" || o.Selection == nil || o.PlanState == nil {
				t.Fatalf("apply=%#v", o)
			}
			id := questionIDInPrompt.FindString(o.Prompt)
			if id == "" {
				t.Fatal("missing fresh capability")
			}
			store, err := askreply.New(storeDir)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.Reply(id, "done"); err != nil {
				t.Fatal(err)
			}
		}
		return dispatch.Report{Session: o.Session, Dir: o.Dir, Agent: "codex", Model: "gpt-6.1-sol", Effort: "medium", Prompt: o.Prompt, DispatchID: "ask-plan-test", PlanState: &dispatch.PlanState{}, Selection: &modelroute.Selection{Status: "selected", Agent: "codex", Model: "gpt-6.1-sol", Effort: "medium"}, Execute: o.Execute}, nil
	}
	out, err := captureRun(t, "ask", "plan-answerer", "--dir", dir, "--store-dir", storeDir, "--auto-model", "--prompt", "investigate", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var r askReport
	if err = json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	if r.PlanFile == "" || r.ApplyCommand == "" {
		t.Fatal("missing automatic plan output")
	}
	defer os.RemoveAll(filepath.Dir(r.PlanFile))
	raw, _ := os.ReadFile(r.PlanFile)
	if strings.Contains(string(raw), "tmact reply protocol") {
		t.Fatal("protocol leaked into saved task")
	}
	out, err = captureRun(t, "ask", "--apply-plan", r.PlanFile, "--execute", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if decisions != 1 || executions != 1 {
		t.Fatalf("decisions=%d executions=%d", decisions, executions)
	}
	if err = json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	if r.Status != "answered" || r.DispatchID != "ask-plan-test" {
		t.Fatalf("answer=%+v", r)
	}
}

func planTestFileMetadata(prompt, dir string) []modelroute.TaskFile {
	f := modelroute.CollectPromptFiles(prompt, dir)
	for i := range f {
		f[i].Content = ""
	}
	return f
}

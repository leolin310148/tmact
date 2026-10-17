package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/leolin310148/tmact/internal/prompt"
	"github.com/leolin310148/tmact/internal/statusd"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// answerCaptureLines is how much of the pane answer_prompt re-reads before it
// presses anything; prompts render at the bottom, so this is generous.
const answerCaptureLines = 200

func addAnswerTool(s *sdk.Server, t tools) {
	sdk.AddTool(s, &sdk.Tool{
		Name: "answer_prompt",
		Description: "Answer the menu a coding agent is showing (choice, command/patch approval, directory access) by pressing one numbered option. " +
			"Copy expected_question and expected_option_label exactly from the prompt in list_panes or wait_pane; the server re-reads the pane and refuses if the prompt changed. " +
			"Workspace-trust prompts are never answered here.",
		Annotations: writeAnnotations,
	}, t.answerPrompt)
}

type promptAnswerIn struct {
	Pane                string `json:"pane" jsonschema:"pane id from list_panes or wait_pane"`
	OptionNumber        int    `json:"option_number" jsonschema:"the numbered option to press"`
	ExpectedQuestion    string `json:"expected_question" jsonschema:"the prompt's observed_question (or observed_title when there is no question), copied verbatim"`
	ExpectedOptionLabel string `json:"expected_option_label" jsonschema:"the chosen option's label without its number, copied verbatim"`
}

type promptAnswerOut struct {
	Pane        string `json:"pane"`
	PromptType  string `json:"prompt_type"`
	Option      int    `json:"option"`
	OptionLabel string `json:"observed_option_label"`
	Next        string `json:"next"`
}

func (t tools) answerPrompt(ctx context.Context, _ *sdk.CallToolRequest, in promptAnswerIn) (*sdk.CallToolResult, promptAnswerOut, error) {
	snap, err := t.snapshot(ctx)
	if err != nil {
		return nil, promptAnswerOut{}, err
	}
	key, pane, ok := findPane(snap, strings.TrimSpace(in.Pane))
	if !ok {
		return nil, promptAnswerOut{}, fmt.Errorf("pane %q is not in the statusd snapshot; use an id from list_panes", in.Pane)
	}
	if !agentRuntimes[pane.Runtime] {
		return nil, promptAnswerOut{}, fmt.Errorf("pane %s runs %q, not a coding agent", key, pane.Runtime)
	}
	peer, paneID := statusd.SplitPeerTarget(pane.PaneID)
	if !localPaneIDPattern.MatchString(paneID) || peer != pane.Peer {
		return nil, promptAnswerOut{}, fmt.Errorf("pane %q has no tmux pane id in the snapshot", key)
	}
	ctx, cancel := context.WithTimeout(ctx, writeCallTimeout)
	defer cancel()

	// Decide from a fresh capture, not the cached snapshot: the menu on screen
	// right now is the one the keypress lands in.
	var raw string
	if peer == "" {
		raw, err = t.deps.CaptureLocal(ctx, paneID, answerCaptureLines)
	} else {
		raw, err = t.deps.CapturePeer(ctx, peer, paneID, answerCaptureLines)
	}
	if err != nil {
		return nil, promptAnswerOut{}, fmt.Errorf("capture %s: %w", key, err)
	}
	detected := prompt.Detect(raw)
	option, err := checkAnswer(detected, in)
	if err != nil {
		return nil, promptAnswerOut{}, fmt.Errorf("pane %s: %w", key, err)
	}
	keys := []string{strconv.Itoa(option.Number)}
	if peer == "" {
		err = t.deps.KeysLocal(ctx, paneID, keys)
	} else {
		err = t.deps.KeysPeer(ctx, peer, paneID, keys)
	}
	if err != nil {
		return nil, promptAnswerOut{}, fmt.Errorf("answer %s: %w", key, err)
	}
	return nil, promptAnswerOut{
		Pane:        key,
		PromptType:  detected.Type,
		Option:      option.Number,
		OptionLabel: option.Label,
		Next:        "Call wait_pane to see what the agent does next.",
	}, nil
}

// checkAnswer is the whole safety decision: a detected, non-trust prompt
// whose question and chosen option still match what the caller saw.
func checkAnswer(detected *prompt.Prompt, in promptAnswerIn) (prompt.Option, error) {
	if detected == nil {
		return prompt.Option{}, fmt.Errorf("no prompt is showing now")
	}
	if detected.Type == prompt.TypeTrustFolder {
		return prompt.Option{}, fmt.Errorf("workspace-trust prompts cannot be answered over MCP; run tmact trust-folder on the host")
	}
	question := detected.Question
	if question == "" {
		question = detected.Title
	}
	if sameText(in.ExpectedQuestion, "") || !sameText(in.ExpectedQuestion, question) {
		return prompt.Option{}, fmt.Errorf("the prompt changed: now showing %q; re-read it with wait_pane before answering", question)
	}
	if in.OptionNumber <= 0 {
		return prompt.Option{}, fmt.Errorf("option_number must be a positive menu number")
	}
	for _, o := range detected.Options {
		if o.Number != in.OptionNumber {
			continue
		}
		if !sameText(in.ExpectedOptionLabel, o.Label) {
			return prompt.Option{}, fmt.Errorf("option %d is now %q, not %q; refusing to answer", o.Number, o.Label, in.ExpectedOptionLabel)
		}
		return o, nil
	}
	return prompt.Option{}, fmt.Errorf("the prompt has no numbered option %d", in.OptionNumber)
}

func sameText(a, b string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(a), " "), strings.Join(strings.Fields(b), " "))
}

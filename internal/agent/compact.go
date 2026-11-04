package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/types"
)

const (
	DefaultReserveTokens    = 16384
	DefaultKeepRecentTokens = 20000
	DefaultSummaryMaxTokens = 8192
	DefaultToolResultChars  = 2000

	PreviousSummaryPrefix = "Previous conversation summary:\n\n"
)

// Settings describes the compaction budget for one chat session.
type Settings struct {
	Enabled          bool
	Window           int // input context window in tokens; 0 = unknown
	ReserveTokens    int
	KeepRecentTokens int
	MaxOutputTokens  int
	SummaryModel     string
	SummaryMaxTokens int
	ToolResultChars  int
}

// withDefaults fills zero values and clamps budgets to the window so a small
// window cannot reserve an unreasonable share of itself.
func (s Settings) withDefaults() Settings {
	if s.ReserveTokens <= 0 {
		s.ReserveTokens = DefaultReserveTokens
	}
	if s.KeepRecentTokens <= 0 {
		s.KeepRecentTokens = DefaultKeepRecentTokens
	}
	if s.SummaryMaxTokens <= 0 {
		s.SummaryMaxTokens = DefaultSummaryMaxTokens
	}
	if s.ToolResultChars <= 0 {
		s.ToolResultChars = DefaultToolResultChars
	}
	if s.Window > 0 {
		// For small windows a flat 16k reserve would consume half the model.
		// Cap it at a quarter (or half for very small windows) while still
		// leaving room for max-output in Threshold().
		maxReserve := s.Window / 4
		if maxReserve < 2048 {
			maxReserve = s.Window / 2
		}
		if s.ReserveTokens > maxReserve {
			s.ReserveTokens = maxReserve
		}
		if s.KeepRecentTokens > s.Window/2 {
			s.KeepRecentTokens = s.Window / 2
		}
		if s.KeepRecentTokens < 512 && s.Window > 2048 {
			s.KeepRecentTokens = 512
		}
	}
	return s
}

// Normalize fills defaults and clamps budgets to the window.
func (s Settings) Normalize() Settings { return s.withDefaults() }

// Threshold returns the context token count that triggers compaction.
func (s Settings) Threshold() int {
	s = s.withDefaults()
	reserve := s.ReserveTokens
	if s.MaxOutputTokens+1024 > reserve {
		reserve = s.MaxOutputTokens + 1024
	}
	if s.Window > 0 && reserve > s.Window/2 {
		reserve = s.Window / 2
	}
	threshold := s.Window - reserve
	if threshold < 1 {
		threshold = 1
	}
	return threshold
}

// ShouldCompact reports whether tokens exceed the configured threshold.
func (s Settings) ShouldCompact(tokens int) bool {
	return s.Enabled && s.Window > 0 && tokens > s.Threshold()
}

// RetryPolicy controls transient-error retries for the summary request.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
}

func (p RetryPolicy) withDefaults() RetryPolicy {
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 3
	}
	if p.BaseDelay < 0 {
		p.BaseDelay = 0
	}
	if p.BaseDelay == 0 {
		p.BaseDelay = 500 * time.Millisecond
	}
	return p
}

func (p RetryPolicy) delay(attempt int) time.Duration {
	if p.BaseDelay <= 0 {
		return 0
	}
	d := p.BaseDelay
	for i := 1; i < attempt; i++ {
		d *= 2
	}
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// CompactOptions carries everything the compactor needs that is not in the
// history itself. Callers pass provider usage and tool-schema overhead so the
// trigger is based on the real request size.
type CompactOptions struct {
	Model              string
	Settings           Settings
	Anchor             *UsageAnchor
	ToolsTokens        int
	CustomInstructions string
	Reason             string
	Force              bool
	Stderr             io.Writer
	Retry              RetryPolicy

	PreviousReadFiles     []string
	PreviousModifiedFiles []string
}

// CompactResult is the new checkpoint plus accounting for persistence.
type CompactResult struct {
	History       []types.ChatMessage
	KeepLast      int
	Summary       string
	Tokens        int // tokens after compaction
	TokensBefore  int
	TokenSaved    int
	Usage         *types.ChatUsage
	ReadFiles     []string
	ModifiedFiles []string
	Reason        string
	SummaryModel  string
	CutIndex      int
	IsSplit       bool
}

// AutoCompact compacts when the context exceeds the threshold. It returns
// (nil, nil) when no compaction is needed and (nil, err) when compaction was
// attempted but failed.
func AutoCompact(ctx context.Context, c *client.Client, history []types.ChatMessage, opts CompactOptions) (*CompactResult, error) {
	s := opts.Settings.withDefaults()
	if (!s.Enabled && !opts.Force) || c == nil {
		return nil, nil
	}
	if s.Window <= 0 && !opts.Force {
		return nil, nil
	}
	before := EstimateContext(history, opts.Anchor, opts.ToolsTokens)
	if !opts.Force && before <= s.Threshold() {
		return nil, nil
	}
	compactLogf(opts.Stderr, "[auto-compact] reason=%s tokens=%d threshold=%d window=%d reserve=%d",
		opts.Reason, before, s.Threshold(), s.Window, s.ReserveTokens)
	return Compact(ctx, c, history, opts)
}

// Compact summarizes older messages and keeps a verbatim recent tail. It is
// also used by manual /compact, so it compacts regardless of the threshold.
func Compact(ctx context.Context, c *client.Client, history []types.ChatMessage, opts CompactOptions) (*CompactResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	s := opts.Settings.withDefaults()
	if c == nil {
		return nil, errors.New("compaction requires a chat client")
	}
	if opts.Reason == "" {
		opts.Reason = "threshold"
	}
	summaryModel := s.SummaryModel
	if summaryModel == "" {
		summaryModel = opts.Model
	}

	before := EstimateContext(history, opts.Anchor, opts.ToolsTokens)
	if s.KeepRecentTokens >= before {
		// The recent budget covers the whole conversation (common for manual
		// /compact and when the window is unknown). Keep a quarter so there is
		// still something to summarize.
		s.KeepRecentTokens = before / 4
		if s.KeepRecentTokens < 100 {
			s.KeepRecentTokens = 0
		}
	}
	system, previousSummary, messages := splitHistory(history)
	if len(messages) == 0 {
		return nil, ErrNothingToCompact
	}

	cut := findCutPoint(messages, s.KeepRecentTokens)
	if cut.firstKept <= 0 && len(messages) > 1 && (opts.Force || before > s.Threshold()) {
		// The provider usage anchor can disagree with the local estimate (for
		// example after an image-heavy turn). If we know the request is over
		// budget, keep the tail from the latest user turn rather than giving up.
		first := lastUserIndex(messages)
		if first <= 0 {
			first = 1
		}
		for first < len(messages) && !validCutTail(messages[first:]) {
			first++
		}
		if first < len(messages) {
			cut = cutPoint{firstKept: first, keepLast: len(messages) - first, isSplit: messages[first].Role != "user"}
		}
	}
	if cut.firstKept <= 0 {
		return nil, ErrNothingToCompact
	}
	toSummarize := messages[:cut.firstKept]
	tail := messages[cut.firstKept:]

	ops := newFileOps()
	ops.addPrevious(opts.PreviousReadFiles, opts.PreviousModifiedFiles)
	ops.addMessages(toSummarize)
	readFiles, modifiedFiles := ops.lists()

	prompt, omitted := buildSummaryInput(toSummarize, previousSummary, s, opts.CustomInstructions, cut.isSplit)
	if omitted > 0 {
		compactLogf(opts.Stderr, "[compact] omitted %d oldest messages to fit the summary request", omitted)
	}
	if s.Window > 0 {
		compactLogf(opts.Stderr, "[compact] cut=%d keep=%d summarize=%d", cut.firstKept, cut.keepLast, len(toSummarize))
	}

	summaryText, usage, err := callSummary(ctx, c, summaryModel, prompt, s, opts.Retry, opts.Stderr)
	if err != nil {
		return nil, err
	}
	summary := stripFileOperationBlocks(summaryText) + formatFileOperations(readFiles, modifiedFiles)
	newHistory := buildProjectedHistory(system, summary, tail)
	after := EstimateContext(newHistory, nil, opts.ToolsTokens)
	if after >= before {
		// A verbose checkpoint can be bigger than the messages it replaces.
		// Ask once for a much more concise version before giving up.
		compactLogf(opts.Stderr, "[compact] summary did not reduce context (%d -> %d); retrying with a concise instruction", before, after)
		concisePrompt := prompt + "\n\nIMPORTANT: The previous summary was too large to reduce the context. Produce a significantly shorter summary while keeping the goal, decisions, file paths and next steps."
		text2, usage2, err2 := callSummary(ctx, c, summaryModel, concisePrompt, s, opts.Retry, opts.Stderr)
		if err2 == nil {
			summary2 := stripFileOperationBlocks(text2) + formatFileOperations(readFiles, modifiedFiles)
			history2 := buildProjectedHistory(system, summary2, tail)
			after2 := EstimateContext(history2, nil, opts.ToolsTokens)
			if after2 < before {
				summary, newHistory, after, usage = summary2, history2, after2, usage2
			}
		}
		if after >= before {
			return nil, fmt.Errorf("%w: %d -> %d tokens", ErrNoReduction, before, after)
		}
	}
	compactLogf(opts.Stderr, "[auto-compact] done %d -> %d tokens (saved %d), kept last %d messages",
		before, after, before-after, cut.keepLast)

	return &CompactResult{
		History:       newHistory,
		KeepLast:      cut.keepLast,
		Summary:       summary,
		Tokens:        after,
		TokensBefore:  before,
		TokenSaved:    before - after,
		Usage:         usage,
		ReadFiles:     readFiles,
		ModifiedFiles: modifiedFiles,
		Reason:        opts.Reason,
		SummaryModel:  summaryModel,
		CutIndex:      cut.firstKept,
		IsSplit:       cut.isSplit,
	}, nil
}

// ErrNothingToCompact means there is no older message that can be summarized
// without dropping the current request.
var ErrNothingToCompact = errors.New("nothing to compact")

// ErrNoReduction means the generated summary would not reduce the context.
var ErrNoReduction = errors.New("summary did not reduce context")

// splitHistory separates leading system prompts, the previous compaction
// summary, and the actual conversation. The previous summary is replaced rather
// than summarized again as ordinary text.
func splitHistory(history []types.ChatMessage) (system []types.ChatMessage, previousSummary string, messages []types.ChatMessage) {
	seenMessage := false
	for _, m := range history {
		if m.Role == "system" && strings.HasPrefix(m.Content, PreviousSummaryPrefix) {
			previousSummary = stripFileOperationBlocks(strings.TrimPrefix(m.Content, PreviousSummaryPrefix))
			continue
		}
		if m.Role == "system" && !seenMessage {
			system = append(system, m)
			continue
		}
		if m.Role == "system" {
			// Stray system notes are not conversation; keep them out of the
			// summary and out of the retained tail.
			continue
		}
		seenMessage = true
		messages = append(messages, m)
	}
	return system, previousSummary, messages
}

// PreviousSummary returns the summary embedded in a projected history.
func PreviousSummary(history []types.ChatMessage) string {
	_, summary, _ := splitHistory(history)
	return summary
}

func buildProjectedHistory(system []types.ChatMessage, summary string, tail []types.ChatMessage) []types.ChatMessage {
	out := make([]types.ChatMessage, 0, len(system)+1+len(tail))
	out = append(out, system...)
	if summary != "" {
		out = append(out, types.ChatMessage{Role: "system", Content: PreviousSummaryPrefix + summary})
	}
	out = append(out, tail...)
	return out
}

// buildSummaryInput serializes the messages to summarize and shrinks the input
// until it fits the model window. The instructions stay intact; only the
// conversation text and the number of old messages are reduced.
func buildSummaryInput(messages []types.ChatMessage, previousSummary string, s Settings, custom string, split bool) (string, int) {
	s = s.withDefaults()
	toolChars := s.ToolResultChars
	budget := 0
	systemTokens := EstimateTokens(summarizationSystemPrompt)
	if s.Window > 0 {
		budget = s.Window - s.SummaryMaxTokens - systemTokens - 2048
		if budget < 256 {
			budget = 256
		}
		// A summary from an earlier compaction must not crowd out the new
		// messages entirely. Keep at most a third of the input budget.
		if maxPrev := budget / 3; maxPrev > 0 && EstimateTokens(previousSummary) > maxPrev {
			previousSummary = truncateForSummary(previousSummary, maxPrev*3)
		}
	}
	omitted := 0
	for {
		conv := serializeConversation(messages, toolChars, 0)
		prompt := buildSummaryPrompt(conv, previousSummary, custom, split)
		if budget <= 0 || EstimateTokens(prompt) <= budget {
			return prompt, omitted
		}
		if toolChars > 500 {
			toolChars = 500
			continue
		}
		if len(messages) > 1 {
			messages = messages[1:]
			omitted++
			continue
		}
		overhead := EstimateTokens(prompt) - EstimateTokens(conv)
		maxConv := budget - overhead
		if maxConv < 256 {
			maxConv = 256
		}
		conv = truncateForSummary(conv, maxConv*3)
		return buildSummaryPrompt(conv, previousSummary, custom, split), omitted
	}
}

func callSummary(ctx context.Context, c *client.Client, model, prompt string, s Settings, retry RetryPolicy, stderr io.Writer) (string, *types.ChatUsage, error) {
	retry = retry.withDefaults()
	maxTokens := s.SummaryMaxTokens
	if s.Window > 0 {
		limit := s.Window - EstimateTokens(prompt) - EstimateTokens(summarizationSystemPrompt) - 1024
		if limit < 128 {
			limit = 128
		}
		if maxTokens > limit {
			maxTokens = limit
		}
	}
	req := &types.ChatRequest{
		Model: model,
		Messages: []types.ChatMessage{
			{Role: "system", Content: summarizationSystemPrompt},
			{Role: "user", Content: prompt},
		},
	}
	if maxTokens > 0 {
		t := maxTokens
		req.MaxTokens = &t
	}

	var lastErr error
	for attempt := 1; attempt <= retry.MaxAttempts; attempt++ {
		resp, err := c.WithContext(ctx).ChatCompletion(req)
		var usage *types.ChatUsage
		if resp != nil {
			usage = resp.Usage
		}
		if err == nil {
			text, verr := validateSummaryResponse(resp)
			if verr == nil {
				return text, usage, nil
			}
			lastErr = verr
			if !retryableSummaryError(verr) {
				return "", usage, verr
			}
		} else {
			lastErr = err
			if !IsTransient(err) {
				return "", usage, err
			}
		}
		if attempt == retry.MaxAttempts {
			break
		}
		compactLogf(stderr, "[compact] summary attempt %d failed: %v; retrying", attempt, lastErr)
		if err := sleepContext(ctx, retry.delay(attempt)); err != nil {
			return "", usage, err
		}
	}
	return "", nil, lastErr
}

var errEmptySummary = errors.New("summary response was empty")

func validateSummaryResponse(resp *types.ChatResponse) (string, error) {
	if resp == nil || len(resp.Choices) == 0 {
		return "", errors.New("summary API returned no choices")
	}
	choice := resp.Choices[0]
	switch choice.FinishReason {
	case "length":
		return "", errors.New("summary generation hit the token cap; refusing to persist an incomplete checkpoint")
	case "tool_calls", "function_call":
		return "", errors.New("summary model attempted to call a tool")
	}
	if len(choice.Message.ToolCalls) > 0 {
		return "", errors.New("summary model attempted to call a tool")
	}
	text := strings.TrimSpace(choice.Message.Content)
	if text == "" {
		return "", errEmptySummary
	}
	return text, nil
}

func retryableSummaryError(err error) bool {
	return errors.Is(err, errEmptySummary)
}

// IsTransient reports whether an error is worth retrying (network errors,
// HTTP 408/429 or 5xx).
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if code, ok := client.StatusCode(err); ok {
		return code == 408 || code == 429 || code >= 500
	}
	var ne net.Error
	return errors.As(err, &ne)
}

// IsContextOverflow reports whether a provider rejected the request because the
// prompt exceeded its context window.
func IsContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"context length", "context_length_exceeded", "maximum context",
		"too many tokens", "reduce the length", "prompt is too long",
		"input is too long", "token limit", "context window",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	if code, ok := client.StatusCode(err); ok {
		return code == 413
	}
	return false
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func compactLogf(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, format+"\r\n", args...)
}

package host

import (
	"context"
	"encoding/json"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

var (
	tskStartSchema  = json.RawMessage(`{"type":"object","properties":{"plan":{"type":"string"},"title":{"type":"string"}},"required":["title","plan"],"additionalProperties":false}`)
	tskSubmitSchema = json.RawMessage(`{"type":"object","properties":{"summary":{"type":"string"}},"required":["summary"],"additionalProperties":false}`)
	tskReviewSchema = json.RawMessage(`{"type":"object","properties":{"items":{"type":"array","items":{"type":"object","properties":{"file":{"type":"string"},"line":{"type":"integer","minimum":1},"text":{"type":"string"},"veto":{"type":"boolean","default":false}},"required":["text"],"additionalProperties":false}},"score":{"type":"integer","minimum":0,"maximum":100}},"required":["score","items"],"additionalProperties":false}`)
)

func init() {
	Register(Tools, "start_task", Tool{
		Description: "Create a direct task for a change to files and move the work to a new task session in this folder. The result says moved, or queued when another task has the folder: the task then starts by itself. End your turn after it.",
		Schema:      tskStartSchema,
		For:         tskForWorkingGroup,
		Call:        tskStartTool,
	})
	Register(Tools, "submit_task", Tool{
		Description: "Submit the task for review. The summary is the report your reviewer reads: what you changed, what you ran and how it ended, and where the work is (the branch and commit that hold it, or this folder only). Nothing in the folder is checked or pushed for you. Call it last, then end your turn.",
		Schema:      tskSubmitSchema,
		For:         tskForTask,
		Call:        tskSubmitTool,
	})
	Register(Tools, "submit_review", Tool{
		Description: "Submit a scored review of the report this session was given, after reading the work where the turn says it is. An item with veto fails the review.",
		Schema:      tskReviewSchema,
		For:         tskForReview,
		Call:        tskReviewTool,
	})
}

func tskForWorkingGroup(c Caller) bool { return c.Kind == GroupSession && !c.Secretary }
func tskForTask(c Caller) bool         { return c.Kind == TaskSession }
func tskForReview(c Caller) bool       { return c.Kind == ReviewSession }

type tskStartArgs struct {
	Title string `json:"title"`
	Plan  string `json:"plan"`
}

// tskStartResult says that the work moved to a task session, or that the
// task is queued behind another one in its folder.
type tskStartResult struct {
	Task   string `json:"task"`
	Moved  bool   `json:"moved,omitempty"`
	Queued bool   `json:"queued,omitempty"`
}

type tskSubmitArgs struct {
	Summary string `json:"summary"`
}

type tskReviewArgs struct {
	Score int                `json:"score"`
	Items []proto.ReviewItem `json:"items"`
}

type tskReviewResult struct {
	Passed bool `json:"passed"`
	Round  int  `json:"round"`
}

func tskDecodeArgs(raw json.RawMessage, dst any) error {
	if proto.Decode(raw, dst) != nil {
		return &Error{Code: "bad_request", Message: "invalid tool arguments"}
	}
	return nil
}

func tskCommand(h *Host, group, as, op string, taskID proto.ID) proto.TaskCommand {
	return proto.TaskCommand{
		HomeCommand: homeHeader(h, group, as, "task"),
		Op:          op,
		ID:          taskID,
	}
}

// tskPost is a post of the caller in the thread of its session's task.
func tskPost[B any](h *Host, c Caller, kind, text string, body B) string {
	taskID := proto.ID(c.Task)
	return marshalWire(proto.Post[B]{
		HomeCommand: secHomeHeader(h, c, "post"),
		Kind:        kind,
		Thread:      &taskID,
		Text:        text,
		Mentions:    []proto.ID{},
		Body:        body,
	})
}

func tskStartTool(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var in tskStartArgs
	if err := tskDecodeArgs(raw, &in); err != nil {
		return nil, err
	}
	if err := CheckOutboundSecret(in.Title); err != nil {
		return nil, err
	}
	if err := CheckOutboundSecret(in.Plan); err != nil {
		return nil, err
	}
	cmd := proto.TaskStart{
		TaskCommand: tskCommand(h, c.Group, c.Member, "start", proto.NewNonce()),
		Title:       in.Title,
		Plan:        in.Plan,
		Re:          nil,
		Origin:      "direct",
	}
	ans, err := h.Command(ctx, c.Home, marshalWire(cmd))
	if err != nil {
		return nil, err
	}
	var res proto.CmdResult
	proto.Decode(ans, &res)
	// The home pushes the row the command made before it answers.
	task, _ := lookupTaskRow(h, c.Group, string(cmd.ID))
	queued := task.State == "queued"
	return tskStartResult{Task: taskLabel(res.Num), Moved: !queued, Queued: queued}, nil
}

// tskSubmitTool submits the task: the summary is posted as a report in the
// task's thread, as the AI wrote it. The node reads no folder
// (contracts/mcp-tools.md section 1).
func tskSubmitTool(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var in tskSubmitArgs
	if err := tskDecodeArgs(raw, &in); err != nil {
		return nil, err
	}
	if err := CheckOutboundSecret(in.Summary); err != nil {
		return nil, err
	}
	task, _ := lookupTaskRow(h, c.Group, c.Task)
	if err := profileRequireAnswered(h.Rows, c.Group, task); err != nil {
		return nil, err
	}
	if _, err := h.Command(ctx, c.Home, tskPost(h, c, "report", in.Summary, proto.EmptyBody{})); err != nil {
		return nil, err
	}
	return secOKReply{OK: true}, nil
}

// tskReviewTool posts the review of the report this session was given when
// its review item was delivered (contracts/storage.md section 3).
func tskReviewTool(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var in tskReviewArgs
	if json.Unmarshal(raw, &in) != nil {
		return nil, &Error{Code: "bad_request", Message: "invalid tool arguments"}
	}
	if err := tskCheckReviewSecrets(in.Items); err != nil {
		return nil, err
	}
	body := proto.ReviewSigned{Task: proto.ID(c.Task), Report: proto.ID(c.Report), Score: in.Score, Items: in.Items}
	ans, err := h.Command(ctx, c.Home, tskPost(h, c, "review", "", body))
	if err != nil {
		return nil, err
	}
	var res tskReviewResult
	json.Unmarshal(ans, &res)
	return res, nil
}

func tskCheckReviewSecrets(items []proto.ReviewItem) error {
	for _, it := range items {
		if err := CheckOutboundSecret(it.Text); err != nil {
			return err
		}
		if it.File != nil {
			if err := CheckOutboundSecret(*it.File); err != nil {
				return err
			}
		}
	}
	return nil
}

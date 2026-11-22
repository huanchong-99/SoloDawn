package proto

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func of[T any]() reflect.Type { return reflect.TypeFor[T]() }

func card[B, S any]() reflect.Type { return of[MessageRow[B, S]]() }

func plain[B any]() reflect.Type { return of[MessageRow[B, NoCard]]() }

// shapes is the file map: the type that reads each wire example, as schemaOf in
// web/src/lib/mock.ts names its schema. A row inside rows, fetched or an inbox
// item, and an envelope inside a signalling frame, is read through its own example.
var shapes = map[string]reflect.Type{
	"ack":                            of[Ack](),
	"act-approve":                    of[Act](),
	"act-choose":                     of[Act](),
	"act-confirm":                    of[Act](),
	"act-deny":                       of[Act](),
	"act-revise":                     of[Act](),
	"agent-set":                      of[AgentSet](),
	"answer":                         of[Answer](),
	"audit-answer":                   of[AuditAnswer](),
	"audit-request":                  of[Audit](),
	"beacon":                         of[Beacon](),
	"cmd":                            of[Cmd](),
	"cmd-error":                      of[ErrorResponse](),
	"cmd-ok":                         of[CmdResult](),
	"cmd-pair-link":                  of[CmdResult](),
	"cmd-reviewed":                   of[CmdResult](),
	"cmd-started":                    of[CmdResult](),
	"device-add":                     of[DeviceEntry](),
	"device-revoke":                  of[DeviceEntry](),
	"devlog":                         of[Devlog](),
	"envelope":                       of[Envelope](),
	"fetch":                          of[FetchThread](),
	"fetch-kind":                     of[FetchKind](),
	"fetch-main":                     of[FetchThread](),
	"fetch-row":                      of[FetchRow](),
	"fetched":                        of[Fetched](),
	"forget":                         of[Forget](),
	"group-create":                   of[GroupCreate](),
	"group-pause":                    of[GroupCommand](),
	"group-resume":                   of[GroupCommand](),
	"group-set":                      of[GroupSet](),
	"groups":                         of[Groups](),
	"hello":                          of[Hello](),
	"host-info-answer":               of[HostInfo](),
	"host-info-push":                 of[HostInfoPush](),
	"host-info-request":              of[Request](),
	"host-join":                      of[HostJoin](),
	"host-leave":                     of[HostLeave](),
	"host-offline":                   of[HostCommand](),
	"host-online":                    of[HostCommand](),
	"ice":                            of[ICECandidate](),
	"inbox":                          of[Inbox](),
	"item-assign":                    of[Item](),
	"item-checks":                    of[Item](),
	"item-confirmed":                 of[Item](),
	"item-decided":                   of[Item](),
	"item-mention":                   of[Item](),
	"item-merge":                     of[Item](),
	"item-notice":                    of[Item](),
	"item-reply":                     of[Item](),
	"item-resume":                    of[Item](),
	"item-review":                    of[Item](),
	"item-reviewed":                  of[Item](),
	"jump":                           of[Jump](),
	"member-add_ai":                  of[MemberAddAI](),
	"member-remove":                  of[MemberCommand](),
	"member-set":                     of[MemberSet](),
	"member-status":                  of[MemberStatus[SystemCode]](),
	"member-status-cap_reached":      of[MemberStatus[CapReachedEvent]](),
	"member-status-login_invalid":    of[MemberStatus[LoginInvalidEvent]](),
	"member-status-rate_limited":     of[MemberStatus[RateLimitedEvent]](),
	"member-status-session_replaced": of[MemberStatus[SessionReplacedEvent]](),
	"member-status-turn_interrupted": of[MemberStatus[TurnInterruptedEvent]](),
	"note":                           of[Note](),
	"offer":                          of[Offer](),
	"pair":                           of[Pair](),
	"pair-browser":                   of[HostCommand](),
	"perm-set":                       of[PermSet](),
	"post-approval":                  of[Post[ApprovalBody]](),
	"post-brief":                     of[Post[BriefBody]](),
	"post-call":                      of[Post[CallBody]](),
	"post-decision":                  of[Post[DecisionBody]](),
	"post-handback":                  of[Post[HandbackBody]](),
	"post-progress":                  of[Post[EmptyBody]](),
	"post-question":                  of[Post[EmptyBody]](),
	"post-reply":                     of[Post[EmptyBody]](),
	"post-report":                    of[Post[ReportBody]](),
	"post-review":                    of[Post[ReviewSigned]](),
	"post-summary":                   of[Post[EmptyBody]](),
	"post-text":                      of[Post[EmptyBody]](),
	"pty-resize":                     of[PTYResize](),
	"push-off":                       of[PushOff](),
	"push-on":                        of[PushOn](),
	"push-reminder":                  of[Reminder](),
	"reset":                          of[Reset](),
	"row-device":                     of[DeviceRow](),
	"row-gone":                       of[GoneRow](),
	"row-group":                      of[GroupRow](),
	"row-group-paused":               of[GroupRow](),
	"row-member-ai":                  of[AIMemberRow](),
	"row-member-human":               of[HumanMemberRow](),
	"row-msg-approval-approved":      card[ApprovalBody, ActedState](),
	"row-msg-approval-denied":        card[ApprovalBody, ActedState](),
	"row-msg-approval-expired":       card[ApprovalBody, TimedState](),
	"row-msg-approval-pending":       card[ApprovalBody, StatusOnly](),
	"row-msg-approval-reassigned":    card[ApprovalBody, TimedState](),
	"row-msg-brief-confirmed":        card[BriefBody, ActedState](),
	"row-msg-brief-pending":          card[BriefBody, StatusOnly](),
	"row-msg-brief-revise":           card[BriefBody, BriefReviseState](),
	"row-msg-brief-superseded":       card[BriefBody, CardState](),
	"row-msg-call-answered":          card[CallBody, CallAnsweredState](),
	"row-msg-call-waiting":           card[CallBody, StatusOnly](),
	"row-msg-checks":                 plain[ChecksBody](),
	"row-msg-decision-decided":       card[DecisionBody, DecisionDecidedState](),
	"row-msg-decision-expired":       card[DecisionBody, TimedState](),
	"row-msg-decision-pending":       card[DecisionBody, StatusOnly](),
	"row-msg-handback":               plain[HandbackBody](),
	"row-msg-progress":               plain[EmptyBody](),
	"row-msg-question":               plain[EmptyBody](),
	"row-msg-reply":                  plain[EmptyBody](),
	"row-msg-report":                 plain[ReportBody](),
	"row-msg-review":                 plain[ReviewBody](),
	"row-msg-summary":                plain[SummaryBody](),
	"row-msg-system":                 plain[TurnInterruptedBody](),
	"row-msg-task":                   plain[TaskBody](),
	"row-msg-text":                   plain[EmptyBody](),
	"row-note":                       of[NoteRow](),
	"row-task":                       of[TaskRow](),
	"row-task-paused":                of[TaskRow](),
	"row-task-waiting":               of[TaskRow](),
	"rows":                           of[Rows](),
	"rv-auth":                        of[Auth](),
	"rv-challenge":                   of[Challenge](),
	"rv-from":                        of[Route](),
	"rv-ice-answer":                  of[ICEConfig](),
	"rv-ice-request":                 of[RVFrame](),
	"rv-join":                        of[RoomID](),
	"rv-joined":                      of[RoomID](),
	"rv-offline":                     of[Offline](),
	"rv-presence":                    of[Presence](),
	"rv-quota-answer":                of[Quota](),
	"rv-quota-request":               of[RVFrame](),
	"rv-ready":                       of[ICEConfig](),
	"rv-room":                        of[RoomData](),
	"rv-to":                          of[Route](),
	"rv-watch":                       of[Watch](),
	"settings-set":                   of[SettingsSet](),
	"sync":                           of[Sync](),
	"synced":                         of[Synced](),
	"takeover-end":                   of[Takeover](),
	"takeover-start":                 of[TakeoverStart](),
	"task-assign":                    of[TaskAssign](),
	"task-branch":                    of[TaskBranch](),
	"task-cancel":                    of[TaskCommand](),
	"task-checked":                   of[TaskChecked](),
	"task-cloud":                     of[TaskChecked](),
	"task-conflict":                  of[TaskConflict](),
	"task-fail":                      of[TaskFail](),
	"task-merged":                    of[TaskMerged](),
	"task-pause":                     of[TaskCommand](),
	"task-preempted":                 of[TaskPreempted](),
	"task-resume":                    of[TaskCommand](),
	"task-start":                     of[TaskStart](),
	"task-submitted":                 of[TaskSubmitted](),
	"trace-local":                    of[TaskTrace](),
	"trace-remote":                   of[TaskTrace](),
	"welcome":                        of[Welcome](),
}

// notExamples are the files of testdata that are not one wire example: test
// vectors, the MCP tool lists, and the facts of wire_test.go.
var notExamples = []string{"crypto", "tools", "wire"}

// exampleNames lists the wire examples in testdata, sorted.
func exampleNames(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, file := range files {
		if name := strings.TrimSuffix(filepath.Base(file), ".json"); !slices.Contains(notExamples, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

func example(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// The web client's golden test fails on a file it has no schema for; so does this one.
func TestEveryExampleHasAType(t *testing.T) {
	typed := []string{}
	for name := range shapes {
		typed = append(typed, name)
	}
	slices.Sort(typed)
	if names := exampleNames(t); !slices.Equal(names, typed) {
		t.Errorf("examples %q\ntypes    %q", names, typed)
	}
}

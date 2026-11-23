package proto

// The payloads of home commands (contracts/commands.md section 1). A receiver
// reads t, and the op, kind or action beside it, before it chooses one of
// these types; the types hold those as text and do not judge them.

// HomeCommand starts the payload of every home command. As is there when a
// node signs for one of its AIs.
type HomeCommand struct {
	Type      string `json:"t"`
	Nonce     ID     `json:"n"`
	Timestamp int64  `json:"ts"`
	Group     ID     `json:"g"`
	As        ID     `json:"as,omitempty"`
}

// GroupCommand is a group command with nothing but its op: pause and resume.
type GroupCommand struct {
	HomeCommand
	Op string `json:"op"`
}

// GroupCreate makes the receiving node the home of a new group, whose id is
// both g and n.
type GroupCreate struct {
	GroupCommand
	Name       string          `json:"name"`
	Kind       string          `json:"kind"`
	Repo       string          `json:"repo,omitempty"`
	Base       string          `json:"base"`
	Forge      string          `json:"forge"`
	Checks     []string        `json:"checks"`
	MemberName string          `json:"member_name"`
	Templates  *ForgeTemplates `json:"templates,omitempty"`
}

// GroupSet carries only the fields being changed; limits, when sent, is whole.
type GroupSet struct {
	GroupCommand
	Name           *string         `json:"name,omitempty"`
	Checks         *[]string       `json:"checks,omitempty"`
	Pass           *int            `json:"pass,omitempty"`
	PassFoundation *int            `json:"pass_foundation,omitempty"`
	Limits         *Limits         `json:"limits,omitempty"`
	Secretary      ID              `json:"secretary,omitempty"`
	Templates      *ForgeTemplates `json:"templates,omitempty"`
}

// MemberCommand names the member a member command is about; alone it is
// remove.
type MemberCommand struct {
	HomeCommand
	Op string `json:"op"`
	ID ID     `json:"id"`
}

type MemberAddAI struct {
	MemberCommand
	CLI       string   `json:"cli"`
	Tool      *string  `json:"tool,omitempty"`
	Name      string   `json:"name"`
	Account   string   `json:"account"`
	Secretary bool     `json:"secretary"`
	Approvers []string `json:"approvers"`
	ProfileSet
}

// MemberStatus carries the fact its host reports as E, one of the events
// below (contracts/commands.md 1.9). Read it as MemberStatus[SystemCode] to
// learn the event's code.
type MemberStatus[E any] struct {
	MemberCommand
	State string `json:"state"`
	Task  ID     `json:"task,omitempty"`
	Path  string `json:"path"`
	Usage Usage  `json:"usage"`
	Event *E     `json:"event,omitempty"`
}

// MemberSet carries only the fields being changed.
type MemberSet struct {
	MemberCommand
	Name      *string   `json:"name,omitempty"`
	Account   *string   `json:"account,omitempty"`
	Tool      *string   `json:"tool,omitempty"`
	Allow     *[]string `json:"allow,omitempty"`
	Approvers *[]string `json:"approvers,omitempty"`
	Role      *string   `json:"role,omitempty"`
	ProfileSet
}

// Invite asks the home for an invitation link for its group; it is a
// HomeCommand alone, answered with link and expires in CmdResult.
type Invite = HomeCommand

// Join admits the person of a joining connection to the invitation's group.
type Join struct {
	HomeCommand
	Name string `json:"name"`
}

// A host event is the body of a system message without ai, which the home
// adds (contracts/messages.md 2.11).

type TurnInterruptedEvent struct {
	SystemCode
	Reason string `json:"reason"`
	Task   ID     `json:"task,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type CapReachedEvent struct {
	SystemCode
	Cap  string `json:"cap"`
	Task ID     `json:"task,omitempty"`
}

type RateLimitedEvent struct {
	SystemCode
	Resets int64 `json:"resets"`
}

type LoginInvalidEvent struct {
	SystemCode
	Detail string `json:"detail,omitempty"`
}

type SessionReplacedEvent struct {
	SystemCode
	Task ID `json:"task,omitempty"`
}

// Post is the message a person or an AI sends; its id is Nonce. B is what the
// author signs of the body of its kind (contracts/messages.md section 2): the
// body type of the row, EmptyBody for a summary, ReviewSigned for a review.
type Post[B any] struct {
	HomeCommand
	Kind     string `json:"kind"`
	Thread   *ID    `json:"thread"`
	Main     bool   `json:"main"`
	Text     string `json:"text"`
	Mentions []ID   `json:"mentions"`
	Re       *ID    `json:"re"`
	Body     B      `json:"body"`
}

// ReviewSigned is the part of a review body that its reviewer signs. The home
// adds the rest and answers with the outcome: passed and round of CmdResult.
type ReviewSigned struct {
	Task   ID           `json:"task"`
	Commit string       `json:"commit"`
	Score  int          `json:"score"`
	Items  []ReviewItem `json:"items"`
}

// ConfirmSigned is what a person signs to ask to watch an AI's terminal
// (contracts/step-2/confirm.md section 3); the home adds to.
type ConfirmSigned struct {
	What string `json:"what"`
	AI   ID     `json:"ai"`
}

// Act decides a card. Which action has arg and text is in
// contracts/commands.md 1.4.
type Act struct {
	HomeCommand
	Msg    ID     `json:"msg"`
	Action string `json:"action"`
	Arg    string `json:"arg,omitempty"`
	Text   string `json:"text,omitempty"`
}

type Jump struct {
	HomeCommand
	Msg ID `json:"msg"`
}

// TaskCommand names the task a task command is about; alone it is pause,
// resume and cancel.
type TaskCommand struct {
	HomeCommand
	Op string `json:"op"`
	ID ID     `json:"id"`
}

type TaskStart struct {
	TaskCommand
	Title  string `json:"title"`
	Plan   string `json:"plan"`
	Re     *ID    `json:"re"`
	Origin string `json:"origin"`
}

type TaskBranch struct {
	TaskCommand
	Branch string `json:"branch"`
}

// TaskChecked is the result of the local checks (checked) and of the hosted
// ones (cloud).
type TaskChecked struct {
	TaskCommand
	OK     bool        `json:"ok"`
	Items  []CheckItem `json:"items"`
	Commit string      `json:"commit"`
}

type TaskSubmitted struct {
	TaskCommand
	Commit string    `json:"commit"`
	PR     int       `json:"pr,omitempty"`
	PRURL  string    `json:"pr_url,omitempty"`
	Stats  DiffStats `json:"stats"`
}

type TaskMerged struct {
	TaskCommand
	Commit      string    `json:"commit"`
	MergeCommit string    `json:"merge_commit"`
	Stats       DiffStats `json:"stats"`
}

// DiffStats are the line counts of a commit against the base.
type DiffStats struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
}

type TaskConflict struct {
	TaskCommand
	Commit string `json:"commit"`
	Detail string `json:"detail"`
}

type TaskFail struct {
	TaskCommand
	Reason string `json:"reason"`
	Detail string `json:"detail"`
}

type TaskPreempted struct {
	TaskCommand
	Msg ID `json:"msg"`
}

// TaskAssign has at least one of its two fields.
type TaskAssign struct {
	TaskCommand
	Assignee ID `json:"assignee,omitempty"`
	Reviewer ID `json:"reviewer,omitempty"`
}

type Note struct {
	HomeCommand
	ID       ID     `json:"id"`
	Task     *ID    `json:"task"`
	NoteType string `json:"type,omitempty"`
	Name     string `json:"name,omitempty"`
	Text     string `json:"text"`
}

type Forget struct {
	HomeCommand
	Row string `json:"r"`
	ID  ID     `json:"id"`
}

// PushOn subscribes the signing browser to reminders.
type PushOn struct {
	HomeCommand
	Sub   PushSubscription `json:"sub"`
	VAPID VAPIDKeys        `json:"vapid"`
	On    []string         `json:"on"`
}

type PushSubscription struct {
	Endpoint string `json:"endpoint"`
	P256DH   string `json:"p256dh"`
	Auth     string `json:"auth"`
}

type VAPIDKeys struct {
	Public  string `json:"pub"`
	Private string `json:"priv"`
}

// PushOff ends the signing browser's reminders. A push whose off is true is
// this command, whatever else it holds; any other is a PushOn.
type PushOff struct {
	HomeCommand
	Off True `json:"off"`
}

// Reminder is what a subscribed browser receives through its push service
// (contracts/commands.md 1.7).
type Reminder struct {
	Group     ID     `json:"g"`
	Msg       ID     `json:"msg"`
	GroupName string `json:"group"`
	Kind      string `json:"kind"`
	Who       string `json:"who"`
	Title     string `json:"title,omitempty"`
}

// HostCommand starts the payload of every host command: To is the node it is
// for (contracts/commands.md section 2).
type HostCommand struct {
	Type      string `json:"t"`
	Nonce     ID     `json:"n"`
	Timestamp int64  `json:"ts"`
	To        string `json:"to"`
}

type HostJoin struct {
	HostCommand
	Group     ID      `json:"g"`
	Home      string  `json:"home"`
	Agent     string  `json:"agent"`
	Name      string  `json:"name"`
	Secretary bool    `json:"secretary"`
	Account   string  `json:"account,omitempty"`
	Tool      *string `json:"tool,omitempty"`
	ProfileSet
}

// HostLeave names the member when the agent has more than one in the group.
type HostLeave struct {
	HostCommand
	Group  ID     `json:"g"`
	Agent  string `json:"agent"`
	Member ID     `json:"member,omitempty"`
}

// HomeAdd records the home of a group on one of a member's nodes before any AI
// on that node joins it (contracts/step-2/people.md section 7).
type HomeAdd struct {
	HostCommand
	Group ID       `json:"g"`
	Home  string   `json:"home"`
	Name  string   `json:"name"`
	Addrs []string `json:"addrs"`
}

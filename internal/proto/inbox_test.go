package proto

import (
	"encoding/json"
	"reflect"
	"testing"
)

// item starts the payload of an item for the example AI, which the home signs.
func item(why string, number int64) Item {
	return Item{Type: "item", Nonce: ID(id("item " + why)), Timestamp: ts, Group: ID(group), To: host, Item: number, AI: ID(agent), Why: why}
}

func (i Item) about(task, msg string) Item {
	i.Task, i.Msg = ID(task), ID(msg)
	return i
}

func TestItemGoldens(t *testing.T) {
	jumped := item("mention", 4401).about("", id("text"))
	jumped.Jump = true
	golden(t, "item-mention", jumped)
	golden(t, "item-reply", item("reply", 4402).about(taskID, id("reply")))
	golden(t, "item-assign", item("assign", 4403).about(taskID, id("task")))
	review := item("review", 4404).about(taskID, "")
	review.AI, review.Commit = ID(reviewer), commit
	golden(t, "item-review", review)
	golden(t, "item-reviewed", item("reviewed", 4405).about(taskID, id("review")))
	golden(t, "item-checks", item("checks", 4406).about(taskID, id("checks")))
	golden(t, "item-resume", item("resume", 4407).about(taskID, ""))
	golden(t, "item-decided", item("decided", 4408).about(taskID, id("decision")))
	confirmed := item("confirmed", 4409).about("", id("brief"))
	confirmed.AI = ID(secretary)
	golden(t, "item-confirmed", confirmed)
	notice := item("notice", 4410).about(taskID, "")
	notice.AI, notice.Code, notice.Detail = ID(secretary), "timeout", "Turn deadline reached."
	golden(t, "item-notice", notice)
	merge := item("merge", 4411).about(taskID, "")
	merge.Commit, merge.PR = commit, 31
	golden(t, "item-merge", merge)
}

func TestInboxGoldens(t *testing.T) {
	assign := item("assign", 4403).about(taskID, id("task"))
	review := item("review", 4404).about(taskID, "")
	review.AI, review.Commit = ID(reviewer), commit
	row := taskExample()
	message := exampleRow[TaskBody, NoCard](t, messageExample{kind: "task", author: "system"}, TaskBody{taskID}, nil, 790)
	confirm := actExample(t, "brief", "confirm", "", "")
	inbox := Inbox{"inbox", ID(group), []InboxItem{
		// The assignment that follows a confirmed brief: its task message, the task, and the act that confirmed.
		{signed(node, string(wireJSON(t, assign))), wireJSON(t, message), &row, &confirm},
		{Env: signed(node, string(wireJSON(t, review))), Task: &row},
	}}
	golden(t, "inbox", inbox)
	golden(t, "ack", Ack{"ack", ID(group), 4403})

	// What a host reads from an item: the payload its home signed, and beside it the rows that payload names.
	var first Item
	if err := Decode([]byte(inbox.Items[0].Env.Payload), &first); err != nil || first != assign {
		t.Fatalf("item payload read as %+v, %v", first, err)
	}
	var copied MessageRow[TaskBody, NoCard]
	if err := Decode(inbox.Items[0].Msg, &copied); err != nil || ID(copied.ID) != first.Msg || ID(inbox.Items[0].Task.ID) != first.Task {
		t.Errorf("the rows beside the item: %+v, %v", copied, err)
	}
}

// An item has jump only after its author's jump, and then it is true.
func TestAnItemJumpsOnlyWithTrue(t *testing.T) {
	plain := item("mention", 1).about("", id("text"))
	encoded := jsonValue(t, wireJSON(t, plain)).(map[string]any)
	if _, there := encoded["jump"]; there {
		t.Errorf("an item that did not jump has jump: %v", encoded)
	}
	for _, jump := range []any{false, "true", 1} {
		encoded["jump"] = jump
		var read Item
		if err := Decode(wireJSON(t, encoded), &read); err == nil {
			t.Errorf("jump %v read as %+v", jump, read)
		}
	}
	encoded["jump"] = true
	var read Item
	if err := Decode(wireJSON(t, encoded), &read); err != nil || !read.Jump {
		t.Errorf("jump true read as %+v, %v", read, err)
	}
}

// An inbox with no items, and an item with nothing beside its envelope, hold no null.
func TestInboxHoldsNoNull(t *testing.T) {
	for _, inbox := range []Inbox{
		{"inbox", ID(group), []InboxItem{}},
		{"inbox", ID(group), []InboxItem{{Env: signed(node, string(wireJSON(t, item("resume", 1).about(taskID, ""))))}}},
	} {
		encoded := wireJSON(t, inbox)
		if nulls := UnexpectedNulls(encoded); len(nulls) != 0 {
			t.Errorf("nulls at %q in %s", nulls, encoded)
		}
		var read Inbox
		if err := Decode(encoded, &read); err != nil || !reflect.DeepEqual(read, inbox) {
			t.Errorf("read %+v, %v\nwant %+v", read, err, inbox)
		}
	}
	var item InboxItem
	for _, member := range []string{"msg", "task", "cause"} {
		if err := Decode([]byte(`{"env":{"p":"{}","k":"k","s":"s"},"`+member+`":null}`), &item); err != errNull {
			t.Errorf("%s null: %v, want %v", member, err, errNull)
		}
	}
	var _ json.RawMessage = item.Msg
}

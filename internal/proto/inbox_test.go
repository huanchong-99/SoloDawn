package proto

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// item starts a plain inbox item for the example AI.
func item(why string, number int64) Item {
	return Item{Item: number, AI: ID(agent), Why: why}
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
	golden(t, "item-watch", item("watch", 4420).about("", id("confirm")))
	notice := item("notice", 4410).about(taskID, "")
	notice.AI, notice.Code, notice.Detail = ID(secretary), "timeout", "Turn deadline reached."
	golden(t, "item-notice", notice)
	merge := item("merge", 4411).about(taskID, "")
	merge.Commit, merge.PR = commit, 31
	golden(t, "item-merge", merge)

	for _, name := range exampleNames(t) {
		if after, ok := strings.CutPrefix(name, "item-"); ok {
			var read Item
			if err := Decode(example(t, name), &read); err != nil || read.Why != after {
				t.Errorf("%s read as why %q, %v", name, read.Why, err)
			}
		}
	}
}

func TestInboxGoldens(t *testing.T) {
	assign := item("assign", 4403).about(taskID, id("task"))
	review := item("review", 4404).about(taskID, "")
	review.AI, review.Commit = ID(reviewer), commit
	row := taskExample()
	message := exampleRow[TaskBody, NoCard](t, messageExample{kind: "task", author: "system"}, TaskBody{taskID}, nil, 790)
	inbox := Inbox{"inbox", ID(group), []InboxItem{
		// The assignment that follows a confirmed brief: the plain item, its task message and the task.
		{assign, wireJSON(t, message), &row},
		{Item: review, Task: &row},
	}}
	golden(t, "inbox", inbox)
	golden(t, "ack", Ack{"ack", ID(group), 4403})

	// What a host reads from an item: the plain item, and beside it the rows that item names.
	var read Inbox
	if err := Decode(example(t, "inbox"), &read); err != nil || read.Items[0].Item != assign {
		t.Fatalf("inbox item read as %+v, %v", read, err)
	}
	var copied MessageRow[TaskBody, NoCard]
	if err := Decode(read.Items[0].Msg, &copied); err != nil || ID(copied.ID) != read.Items[0].Item.Msg || ID(read.Items[0].Task.ID) != read.Items[0].Item.Task {
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

// An inbox with no items, and an item with nothing beside its plain item, hold no null.
func TestInboxHoldsNoNull(t *testing.T) {
	resume := item("resume", 1).about(taskID, "")
	for _, inbox := range []Inbox{
		{"inbox", ID(group), []InboxItem{}},
		{"inbox", ID(group), []InboxItem{{Item: resume}}},
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
	raw := string(wireJSON(t, resume))
	var got InboxItem
	if err := Decode([]byte(`{"item":null}`), &got); err != errNull {
		t.Errorf("item null: %v, want %v", err, errNull)
	}
	for _, member := range []string{"msg", "task"} {
		if err := Decode([]byte(`{"item":`+raw+`,"`+member+`":null}`), &got); err != errNull {
			t.Errorf("%s null: %v, want %v", member, err, errNull)
		}
	}
	var _ json.RawMessage = got.Msg
}

package model

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"
)

// TestTodoInputJSONExposesNoCredentials is the leak guard for the task detail
// endpoint: handler.TaskHandler.Get serialises model.TaskDetail directly
// (transport.WriteData(c, http.StatusOK, task)), so EVERY field added to
// TodoInput becomes browser-visible to anyone who can read the task.
//
// protocol.TodoInputRef — the sibling struct that goes to the executing agent —
// carries DownloadUrl (a signed, time-limited JWT) and FileRef (an internal
// file id). Neither may cross into the browser shape. The exclusion is enforced
// by the type itself rather than by a converter, so this test is what keeps it
// true: any new field fails the allow-list until someone deliberately adds it
// there, and that edit is the review.
func TestTodoInputJSONExposesNoCredentials(t *testing.T) {
	checkedAt := time.Date(2026, 9, 29, 5, 12, 24, 0, time.UTC)
	payload, err := json.Marshal(TodoInput{
		Name:       "分组视频生成提示词",
		SourceStep: "分镜分组",
		OutputName: "分组视频生成提示词",
		FileName:   "prompt.md",
		FileSize:   51382,
		State:      InputStateResolved,
		CheckedAt:  &checkedAt,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	got := make([]string, 0, len(raw))
	for k := range raw {
		got = append(got, k)
	}
	sort.Strings(got)

	want := []string{"checked_at", "file_name", "file_size", "name", "output_name", "source_step", "state"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TodoInput JSON keys = %v, want %v\n"+
			"Every field here is served to the browser on the task detail endpoint.\n"+
			"Confirm the new one is neither a credential nor an internal handle, then extend this list.",
			got, want)
	}

	// Belt and braces: name the sensitive keys explicitly, so a future rewrite
	// of the allow-list above cannot quietly re-admit them.
	for _, forbidden := range []string{"download_url", "file_ref", "artifact_id", "source_todo_id"} {
		if _, ok := raw[forbidden]; ok {
			t.Errorf("%q must never reach the browser: %s", forbidden, payload)
		}
	}
}

// TestTodoInputStateIsAlwaysSerialised covers both ends of the omitempty policy
// in one place: the optional fields vanish when unset (so a todo observed
// before those fields existed gains nothing on the wire), while `state` is
// never omitted — an absent state would read as "this step has no inputs"
// rather than "checked, and it is unresolved", which is the ambiguity the field
// exists to remove.
func TestTodoInputStateIsAlwaysSerialised(t *testing.T) {
	payload, err := json.Marshal(TodoInput{Name: "上一段剧本", State: InputStateMissing})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(payload) != `{"name":"上一段剧本","state":"missing"}` {
		t.Fatalf("minimal TodoInput = %s, want only name+state", payload)
	}
}

// TestTodoOmitsInputsWhenNeverObserved pins the wire-compatibility half of
// adding the field: a todo that has never been through a dispatch keeps its
// previous JSON shape exactly, so no existing client sees a new key until there
// is something to say.
func TestTodoOmitsInputsWhenNeverObserved(t *testing.T) {
	payload, err := json.Marshal(Todo{ID: "TD_01", Status: "pending"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := raw["inputs"]; ok {
		t.Fatalf("a never-observed todo must not carry an inputs key: %s", payload)
	}
	// The symmetric field must keep behaving identically — this whole change
	// adds the input-side mirror of Outputs, so a divergence between the two
	// omitempty policies would be a bug in itself.
	if _, ok := raw["outputs"]; ok {
		t.Fatalf("outputs and inputs must share the same omitempty policy: %s", payload)
	}
}

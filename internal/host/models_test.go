package host

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestModelTiers(t *testing.T) {
	for id, want := range map[string]string{
		"gemini-3-pro": "standard", "gpt-6-mini": "light", "MINI.opus": "light",
		"minimax": "standard", "claude/OPUS_5": "strong", "fable": "strong",
		"haiku": "light", "nano": "light", "flash": "light", "lite": "light", "": "standard",
		"émini": "standard", "opus²": "strong",
	} {
		agtEqual(t, modelTier(id), want)
	}
	for _, row := range modelTiers {
		agtEqual(t, row.source, "planning/contracts/step-3/drivers.md#102-the-suggested-tier")
	}
}

func modelReader(text string) *bufio.Reader { return bufio.NewReader(strings.NewReader(text)) }

const claudeModelReply = `{"type":"control_response","response":{"subtype":"success","request_id":"models","response":{"models":[{"value":"default","resolvedModel":"claude-opus-5","displayName":"Default","supportsEffort":true,"supportedEffortLevels":["low","high"],"supportsFastMode":true},{"value":"haiku","resolvedModel":"claude-haiku-5","displayName":"Haiku","supportsEffort":false}]}}}`

func modelWantClaude() []proto.Model {
	return []proto.Model{{Model: "default", Name: "Default", Efforts: []string{"low", "high"}, Fast: true, Tier: "strong"}, {Model: "haiku", Name: "Haiku", Efforts: []string{}, Tier: "light"}}
}

func modelWantCodex() []proto.Model {
	return []proto.Model{{Model: "gpt-6-mini", Name: "Mini", Efforts: []string{"medium", "high"}, Fast: true, Tier: "light"}, {Model: "gpt-6-luna", Name: "Luna", Efforts: []string{}, Tier: "standard"}}
}

func TestClaudeModelsReadFieldsAndDefaults(t *testing.T) {
	var sent bytes.Buffer
	got, err := readClaudeModels(modelReader(claudeModelReply+"\n"), json.NewEncoder(&sent))
	agtCheck(t, err)
	agtEqual(t, got, modelWantClaude())
	agtEqual(t, sent.String(), "{\"type\":\"control_request\",\"request_id\":\"models\",\"request\":{\"subtype\":\"initialize\"}}\n")
	got, err = claudeModelList([]claudeModel{{Value: "id", SupportedEffortLevels: []string{"ignored"}}})
	agtCheck(t, err)
	agtEqual(t, got, []proto.Model{{Model: "id", Name: "id", Efforts: []string{}, Tier: "standard"}})
	got, err = claudeModelList([]claudeModel{})
	agtCheck(t, err)
	agtEqual(t, got, []proto.Model{})
}

func TestClaudeModelsRejectUnknownShapes(t *testing.T) {
	for _, raw := range []string{
		"", "{}\n", "{\n", claudeModelReply,
		strings.Replace(claudeModelReply, "control_response", "other", 1) + "\n",
		strings.Replace(claudeModelReply, "success", "error", 1) + "\n",
		strings.Replace(claudeModelReply, `"request_id":"models"`, `"request_id":"other"`, 1) + "\n",
		`{"type":"control_response","response":{"subtype":"success","request_id":"models","response":{}}}` + "\n",
		strings.Replace(claudeModelReply, `"value":"default"`, `"value":null`, 1) + "\n",
		strings.Replace(claudeModelReply, `"supportsEffort":true`, `"supportsEffort":"yes"`, 1) + "\n",
		strings.Replace(claudeModelReply, `"supportedEffortLevels":["low","high"]`, `"supportedEffortLevels":null`, 1) + "\n",
		strings.Replace(claudeModelReply, `"supportedEffortLevels":["low","high"]`, `"supportedEffortLevels":[null]`, 1) + "\n",
	} {
		_, err := readClaudeModels(modelReader(raw), json.NewEncoder(io.Discard))
		if err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}

type modelWriteError struct{}

func (modelWriteError) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

type modelFailAfter struct{ left int }

func (w *modelFailAfter) Write(b []byte) (int, error) {
	if w.left == 0 {
		return 0, io.ErrClosedPipe
	}
	w.left--
	return len(b), nil
}

func TestModelRequestWriteFailures(t *testing.T) {
	_, err := readClaudeModels(modelReader(""), json.NewEncoder(modelWriteError{}))
	agtEqual(t, err, io.ErrClosedPipe)
	_, err = readCodexModels(modelReader(""), json.NewEncoder(modelWriteError{}))
	agtEqual(t, err, io.ErrClosedPipe)
	_, err = readCodexModels(modelReader("{\"id\":1,\"result\":{}}\n"), json.NewEncoder(&modelFailAfter{left: 1}))
	agtEqual(t, err, io.ErrClosedPipe)
}

func TestCodexModelShapes(t *testing.T) {
	for _, raw := range []string{
		"", "{}\n", "{\n", `{"id":9,"result":{}}` + "\n", `{"id":2,"error":{"message":"no"}}` + "\n",
		`{"id":2,"result":{}}` + "\n", `{"id":2,"result":{"data":[{}],"nextCursor":null}}` + "\n",
		`{"id":2,"result":{"data":[{"model":"id","serviceTiers":[]}],"nextCursor":null}}` + "\n",
		`{"id":2,"result":{"data":[{"model":"id","supportedReasoningEfforts":[]}],"nextCursor":null}}` + "\n",
		`{"id":2,"result":{"data":[{"model":"id","supportedReasoningEfforts":[{}],"serviceTiers":[]}],"nextCursor":null}}` + "\n",
	} {
		_, err := codexModelPages(modelReader(raw), json.NewEncoder(io.Discard))
		if err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	got, err := codexModelPages(modelReader(`{"id":2,"result":{"data":[],"nextCursor":null}}`+"\n"), json.NewEncoder(io.Discard))
	agtCheck(t, err)
	agtEqual(t, got, []proto.Model{})
	agtEqual(t, codexListedModel(codexModel{Model: "id"}), proto.Model{Model: "id", Name: "id", Efforts: []string{}, Tier: "standard"})
}

func TestCodexModelNotificationsAndSecondPageFailure(t *testing.T) {
	var got map[string]string
	err := readModelRPC(modelReader("{\"method\":\"notice\"}\n{\"id\":3,\"result\":{\"status\":\"ok\"}}\n"), 3, &got)
	agtCheck(t, err)
	agtEqual(t, got, map[string]string{"status": "ok"})
	page := `{"id":2,"result":{"data":[{"model":"gpt-6-mini","supportedReasoningEfforts":[],"serviceTiers":[]}],"nextCursor":"next"}}`
	models, err := codexModelPages(modelReader(page+"\n{\"id\":3,\"result\":{}}\n"), json.NewEncoder(io.Discard))
	agtErrText(t, err, "codex: missing model list")
	agtEqual(t, models, []proto.Model(nil))
}

func TestModelProbeCancellationWhileReading(t *testing.T) {
	modelBinaries(t)
	modelScript(t, "claude", `{"session":"models","steps":[{"dir":"stdin","msg":{"type":"control_request"}},{"dir":"stdin","msg":{"wait":true}}]}`)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	read := func(from *bufio.Reader, to *json.Encoder) ([]proto.Model, error) {
		cancel()
		return readClaudeModels(from, to)
	}
	agtEqual(t, probeModels(ctx, "claude", nil, read), []proto.Model{})
	agtEqual(t, ctx.Err(), context.Canceled)
}

func modelBinaries(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out, err := exec.Command("go", "build", "-o", dir+string(os.PathSeparator), "./fakecli/claude", "./fakecli/codex").CombinedOutput()
	if err != nil {
		t.Fatalf("build fake CLIs: %s: %v", out, err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SOLODAWN_FAKECLI_LOG", filepath.Join(t.TempDir(), "log"))
	return dir
}

func modelScript(t *testing.T, agent, text string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "script.json")
	agtCheck(t, os.WriteFile(path, []byte(text), 0600))
	t.Setenv("SOLODAWN_FAKE_"+strings.ToUpper(agent)+"_SCRIPT", path)
}

func TestModelDiscoveryWithFakeCLIs(t *testing.T) {
	modelBinaries(t)
	for agent, want := range map[string][]proto.Model{"claude": modelWantClaude(), "codex": modelWantCodex()} {
		t.Setenv("SOLODAWN_FAKE_"+strings.ToUpper(agent)+"_SCRIPT", filepath.Join("fakecli", "testdata", agent+"-models.json"))
		agtEqual(t, agtModels(t.Context(), agent), want)
		modelScript(t, agent, `{"session":"models","steps":[],"exit":1}`)
		agtEqual(t, agtModels(t.Context(), agent), []proto.Model{})
	}
	modelScript(t, "claude", `{"session":"models","steps":[{"dir":"stdout","msg":`+claudeModelReply+`},{"dir":"stdin","msg":{"type":"control_request"}},{"dir":"eof"}],"exit":1}`)
	agtEqual(t, agtModels(t.Context(), "claude"), []proto.Model{})
	agtEqual(t, agtModels(t.Context(), "opencode"), []proto.Model{})
	agtEqual(t, agtModels(t.Context(), "qwen"), []proto.Model{})
	agtEqual(t, agtModels(t.Context(), "absent"), []proto.Model{})
}

func TestModelProbeFailures(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())
	stop()
	agtEqual(t, probeModels(ctx, "absent-model-cli", nil, readClaudeModels), []proto.Model{})
	modelBinaries(t)
	modelScript(t, "claude", `{"session":"models","steps":[{"dir":"eof"}]}`)
	readFail := func(*bufio.Reader, *json.Encoder) ([]proto.Model, error) { return nil, errors.New("read failed") }
	agtEqual(t, probeModels(t.Context(), "claude", nil, readFail), []proto.Model{})
}

func TestModelDiscoveryCacheAndRefresh(t *testing.T) {
	dir := modelBinaries(t)
	t.Setenv("PATH", agtIsolatedPath(dir))
	modelScript(t, "claude", `{"session":"models","steps":[{"dir":"stdin","msg":{"type":"control_request"}},{"dir":"stdout","msg":`+claudeModelReply+`},{"dir":"eof"}]}`)
	modelScript(t, "codex", `{"session":"models","exit":1}`)
	h, _ := seamHost(t, seamStore(t))
	now := time.UnixMilli(1763600000000)
	h.Now = func() time.Time { return now }
	first, err := h.Agents(t.Context(), false)
	agtCheck(t, err)
	agtEqual(t, first[0].Models, modelWantClaude())
	modelScript(t, "claude", `{"session":"models","exit":1}`)
	now = now.Add(59 * time.Second)
	cached, err := h.Agents(t.Context(), false)
	agtCheck(t, err)
	agtEqual(t, cached, first)
	fresh, err := h.Agents(t.Context(), true)
	agtCheck(t, err)
	first[0].Models = []proto.Model{}
	agtEqual(t, fresh, first)
	now = now.Add(time.Minute)
	again, err := h.Agents(t.Context(), false)
	agtCheck(t, err)
	agtEqual(t, again, fresh)
}

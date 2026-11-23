package host

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"slices"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func agtModels(ctx context.Context, agent string) []proto.Model {
	if driver, ok := Drivers[agent].(interface {
		Models(context.Context) []proto.Model
	}); ok {
		return driver.Models(ctx)
	}
	return []proto.Model{}
}

func probeModels(ctx context.Context, agent string, args []string, read func(*bufio.Reader, *json.Encoder) ([]proto.Model, error)) []proto.Model {
	ctx, cancel := context.WithTimeout(ctx, agtProbeSeconds*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, agent, args...)
	detachTerminal(cmd)
	cmd.WaitDelay = commandWaitDelay
	in, out, err := startCodexCmd(cmd)
	if err != nil {
		return []proto.Model{}
	}
	defer killCheckTree(cmd)()
	stop := context.AfterFunc(ctx, func() { out.Close() })
	defer stop()
	models, err := read(bufio.NewReader(out), json.NewEncoder(in))
	if err != nil {
		cancel()
	}
	in.Close()
	waitErr := cmd.Wait()
	out.Close()
	if err != nil || waitErr != nil {
		return []proto.Model{}
	}
	return models
}

type claudeModel struct {
	Value                 string   `json:"value"`
	DisplayName           string   `json:"displayName,omitempty"`
	ResolvedModel         string   `json:"resolvedModel,omitempty"`
	SupportsEffort        bool     `json:"supportsEffort,omitempty"`
	SupportedEffortLevels []string `json:"supportedEffortLevels,omitempty"`
	SupportsFastMode      bool     `json:"supportsFastMode,omitempty"`
}

func readClaudeModels(from *bufio.Reader, to *json.Encoder) ([]proto.Model, error) {
	err := to.Encode(claudeControlReq{"control_request", "models", claudeControlSubtype{"initialize"}})
	if err != nil {
		return nil, err
	}
	var reply struct {
		Type     string `json:"type"`
		Response struct {
			Subtype   string `json:"subtype"`
			RequestID string `json:"request_id"`
			Response  struct {
				Models []claudeModel `json:"models"`
			} `json:"response"`
		} `json:"response"`
	}
	if err := readModelFrame(from, &reply); err != nil {
		return nil, err
	}
	if reply.Type != "control_response" || reply.Response.Subtype != "success" || reply.Response.RequestID != "models" {
		return nil, errors.New("claude: invalid model listing response")
	}
	return claudeModelList(reply.Response.Response.Models)
}

func claudeModelList(list []claudeModel) ([]proto.Model, error) {
	models := []proto.Model{}
	if list == nil {
		return nil, errors.New("claude: missing model list")
	}
	for _, m := range list {
		if !validClaudeModel(m) {
			return nil, errors.New("claude: invalid model entry")
		}
		models = append(models, claudeListedModel(m))
	}
	return models, nil
}

func validClaudeModel(m claudeModel) bool {
	return m.Value != "" && (!m.SupportsEffort || m.SupportedEffortLevels != nil) && !slices.Contains(m.SupportedEffortLevels, "")
}

func claudeListedModel(m claudeModel) proto.Model {
	efforts := []string{}
	if m.SupportsEffort {
		efforts = append(efforts, m.SupportedEffortLevels...)
	}
	return proto.Model{Model: m.Value, Name: cmp.Or(m.DisplayName, m.Value), Efforts: efforts, Fast: proto.True(m.SupportsFastMode), Tier: modelTier(m.ResolvedModel)}
}

type codexModel struct {
	Model                     string `json:"model"`
	DisplayName               string `json:"displayName,omitempty"`
	SupportedReasoningEfforts []struct {
		ReasoningEffort string `json:"reasoningEffort"`
	} `json:"supportedReasoningEfforts"`
	ServiceTiers []json.RawMessage `json:"serviceTiers"`
}

func readCodexModels(from *bufio.Reader, to *json.Encoder) ([]proto.Model, error) {
	init := codexRPCRequest{ID: 1, Method: "initialize", Params: map[string]any{"clientInfo": map[string]string{"name": "solodawn", "version": "1"}}}
	var initialized map[string]json.RawMessage
	if err := modelRPC(from, to, init, &initialized); err != nil {
		return nil, err
	}
	if err := to.Encode(map[string]string{"method": "initialized"}); err != nil {
		return nil, err
	}
	return codexModelPages(from, to)
}

func codexModelPages(from *bufio.Reader, to *json.Encoder) ([]proto.Model, error) {
	models := []proto.Model{}
	params := map[string]any{"includeHidden": false}
	for id := int64(2); ; id++ {
		var page struct {
			Data       []codexModel `json:"data"`
			NextCursor *string      `json:"nextCursor"`
		}
		if err := modelRPC(from, to, codexRPCRequest{ID: id, Method: "model/list", Params: params}, &page); err != nil {
			return nil, err
		}
		if err := checkCodexModels(page.Data); err != nil {
			return nil, err
		}
		for _, m := range page.Data {
			models = append(models, codexListedModel(m))
		}
		if page.NextCursor == nil {
			return models, nil
		}
		params["cursor"] = *page.NextCursor
	}
}

func checkCodexModels(models []codexModel) error {
	if models == nil {
		return errors.New("codex: missing model list")
	}
	for _, m := range models {
		if !validCodexModel(m) {
			return errors.New("codex: invalid model entry")
		}
	}
	return nil
}

func validCodexModel(m codexModel) bool {
	if m.Model == "" || m.SupportedReasoningEfforts == nil || m.ServiceTiers == nil {
		return false
	}
	for _, level := range m.SupportedReasoningEfforts {
		if level.ReasoningEffort == "" {
			return false
		}
	}
	return true
}

func codexListedModel(m codexModel) proto.Model {
	efforts := []string{}
	for _, level := range m.SupportedReasoningEfforts {
		efforts = append(efforts, level.ReasoningEffort)
	}
	return proto.Model{Model: m.Model, Name: cmp.Or(m.DisplayName, m.Model), Efforts: efforts, Fast: proto.True(len(m.ServiceTiers) > 0), Tier: modelTier(m.Model)}
}

func modelRPC(from *bufio.Reader, to *json.Encoder, request codexRPCRequest, result any) error {
	if err := to.Encode(request); err != nil {
		return err
	}
	return readModelRPC(from, request.ID, result)
}

func readModelRPC(from *bufio.Reader, id int64, result any) error {
	for {
		var reply struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
		}
		if err := readModelFrame(from, &reply); err != nil {
			return err
		}
		if reply.Method != "" {
			continue
		}
		if reply.ID == nil || *reply.ID != id {
			return errors.New("codex: invalid model listing response id")
		}
		return json.Unmarshal(reply.Result, result)
	}
}

func readModelFrame(from *bufio.Reader, result any) error {
	line, err := from.ReadBytes('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(line, result)
}

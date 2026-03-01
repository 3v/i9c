package codexbridge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

func DiscoverModels(ctx context.Context, command string) ([]string, error) {
	if command == "" {
		command = "codex"
	}

	cmd := execCommandContext(ctx, command, "app-server")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	go io.Copy(io.Discard, stderr)

	writeJSON := func(v any) error {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		_, err = stdin.Write(append(data, '\n'))
		return err
	}

	reader := bufio.NewScanner(stdout)
	waitResp := func(targetID int) (json.RawMessage, error) {
		timeout := time.NewTimer(20 * time.Second)
		defer timeout.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timeout.C:
				return nil, fmt.Errorf("timeout waiting response id %d", targetID)
			default:
				if !reader.Scan() {
					if err := reader.Err(); err != nil {
						return nil, err
					}
					return nil, fmt.Errorf("codex app-server closed")
				}
				line := strings.TrimSpace(reader.Text())
				if line == "" {
					continue
				}
				var msg struct {
					ID     json.RawMessage `json:"id"`
					Method string          `json:"method"`
					Result json.RawMessage `json:"result"`
					Error  json.RawMessage `json:"error"`
				}
				if err := json.Unmarshal([]byte(line), &msg); err != nil {
					continue
				}
				// If server asks for a request during handshake/list, decline it.
				if len(msg.ID) > 0 && msg.Method != "" {
					_ = writeJSON(map[string]any{
						"id":    msg.ID,
						"error": map[string]any{"code": -32601, "message": "unsupported in model discovery"},
					})
					continue
				}
				if len(msg.ID) == 0 || msg.Method != "" {
					continue
				}
				var id int
				if err := json.Unmarshal(msg.ID, &id); err != nil || id != targetID {
					continue
				}
				if len(msg.Error) > 0 {
					return nil, fmt.Errorf("rpc error: %s", string(msg.Error))
				}
				return msg.Result, nil
			}
		}
	}

	if err := writeJSON(map[string]any{
		"id":     1,
		"method": "initialize",
		"params": map[string]any{
			"clientInfo": map[string]any{"name": "i9c", "title": "i9c", "version": "0.1.0"},
		},
	}); err != nil {
		return nil, err
	}
	if _, err := waitResp(1); err != nil {
		return nil, err
	}
	_ = writeJSON(map[string]any{"method": "initialized", "params": map[string]any{}})

	if err := writeJSON(map[string]any{
		"id":     2,
		"method": "model/list",
		"params": map[string]any{"includeHidden": false},
	}); err != nil {
		return nil, err
	}

	result, err := waitResp(2)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data []struct {
			ID        string `json:"id"`
			Model     string `json:"model"`
			IsDefault bool   `json:"isDefault"`
		} `json:"data"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		return nil, err
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("no models returned")
	}

	seen := map[string]bool{}
	defaultModels := []string{}
	others := []string{}
	for _, m := range payload.Data {
		name := strings.TrimSpace(m.Model)
		if name == "" {
			name = strings.TrimSpace(m.ID)
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if m.IsDefault {
			defaultModels = append(defaultModels, name)
		} else {
			others = append(others, name)
		}
	}
	sort.Strings(others)
	return append(defaultModels, others...), nil
}

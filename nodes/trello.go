package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"orchkit"
)

// Trello manages boards, lists, and cards via Trello REST API.
// Actions: list_boards, list_cards, create_card, update_card,
//          move_card, delete_card, create_list.
//
// Example:
//
//	nodes.NewTrello("api_key", "token")
type Trello struct {
	APIKey string
	Token  string
	client *http.Client
}

func NewTrello(apiKey, token string) *Trello {
	return &Trello{APIKey: apiKey, Token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (t *Trello) Name() string { return "trello" }

func (t *Trello) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Manages Trello boards, lists, and cards. Actions: list_boards, list_cards, create_card, update_card, move_card, delete_card.",
		Params: map[string]any{
			"action":   map[string]any{"type": "string", "desc": "list_boards | list_cards | create_card | update_card | move_card | delete_card"},
			"board_id": map[string]any{"type": "string", "desc": "Board ID (list_cards, create_card)."},
			"list_id":  map[string]any{"type": "string", "desc": "List ID (create_card, move_card)."},
			"card_id":  map[string]any{"type": "string", "desc": "Card ID (update_card, move_card, delete_card)."},
			"name":     map[string]any{"type": "string", "desc": "Card or list name."},
			"desc":     map[string]any{"type": "string", "desc": "Card description."},
		},
	}
}

func (t *Trello) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	base := "https://api.trello.com/1"
	auth := fmt.Sprintf("key=%s&token=%s", t.APIKey, t.Token)
	action, _ := in["action"].(string)

	switch action {
	case "list_boards", "":
		return t.get(ctx, base+"/members/me/boards?"+auth)

	case "list_cards":
		boardID, _ := in["board_id"].(string)
		if boardID == "" {
			return nil, fmt.Errorf("trello: board_id required")
		}
		return t.get(ctx, fmt.Sprintf("%s/boards/%s/cards?%s", base, boardID, auth))

	case "create_card":
		listID, _ := in["list_id"].(string)
		name, _ := in["name"].(string)
		if listID == "" || name == "" {
			return nil, fmt.Errorf("trello: list_id and name required")
		}
		body := map[string]any{"idList": listID, "name": name}
		if desc, ok := in["desc"].(string); ok {
			body["desc"] = desc
		}
		return t.post(ctx, base+"/cards?"+auth, body)

	case "update_card":
		cardID, _ := in["card_id"].(string)
		if cardID == "" {
			return nil, fmt.Errorf("trello: card_id required")
		}
		body := map[string]any{}
		if name, ok := in["name"].(string); ok {
			body["name"] = name
		}
		if desc, ok := in["desc"].(string); ok {
			body["desc"] = desc
		}
		return t.put(ctx, fmt.Sprintf("%s/cards/%s?%s", base, cardID, auth), body)

	case "move_card":
		cardID, _ := in["card_id"].(string)
		listID, _ := in["list_id"].(string)
		if cardID == "" || listID == "" {
			return nil, fmt.Errorf("trello: card_id and list_id required")
		}
		return t.put(ctx, fmt.Sprintf("%s/cards/%s?%s", base, cardID, auth),
			map[string]any{"idList": listID})

	case "delete_card":
		cardID, _ := in["card_id"].(string)
		if cardID == "" {
			return nil, fmt.Errorf("trello: card_id required")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
			fmt.Sprintf("%s/cards/%s?%s", base, cardID, auth), nil)
		if err != nil {
			return nil, err
		}
		return t.do(req)

	default:
		return nil, fmt.Errorf("trello: unknown action %q", action)
	}
}

func (t *Trello) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return t.do(req)
}

func (t *Trello) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return t.do(req)
}

func (t *Trello) put(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return t.do(req)
}

func (t *Trello) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("content-type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trello: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("trello: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}

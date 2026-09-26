package client

import (
	"bytes"
	"encoding/json"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// zdrDataCollectionDeny is OpenRouter's value for refusing providers that may
// store user data. Sent alongside "zdr": true.
const zdrDataCollectionDeny = "deny"

// ApplyOpenRouterZDR merges the OpenRouter zero-data-retention routing object
// ({"provider":{"zdr":true,"data_collection":"deny"}}) into a marshalled JSON
// object body. It is a no-op unless zdr is true, baseURL is OpenRouter, and the
// body is a JSON object. Existing "provider" sub-fields are preserved.
//
// OpenRouter only honors these fields on chat-completions-family endpoints
// (chat/completions, messages, responses). Its Images and Videos APIs reject or
// ignore them and video is ineligible for ZDR entirely, so callers must never
// apply this to those request bodies.
func ApplyOpenRouterZDR(body []byte, zdr bool, baseURL string) ([]byte, error) {
	if !zdr || !provider.IsOpenRouter(baseURL) || len(body) == 0 {
		return body, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return body, nil
	}
	prefs := map[string]any{}
	if raw, ok := obj["provider"]; ok {
		_ = json.Unmarshal(raw, &prefs)
		if prefs == nil {
			prefs = map[string]any{}
		}
	}
	prefs["zdr"] = true
	prefs["data_collection"] = zdrDataCollectionDeny
	merged, err := json.Marshal(prefs)
	if err != nil {
		return nil, err
	}
	obj["provider"] = merged

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(obj); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// ChatBody marshals a chat request and applies the OpenRouter ZDR routing
// object when enabled. Shared by the real ChatCompletion call and the --dry-run
// curl renderer so the preview cannot drift from what is sent.
func ChatBody(req *types.ChatRequest, zdr bool, baseURL string) ([]byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return ApplyOpenRouterZDR(body, zdr, baseURL)
}

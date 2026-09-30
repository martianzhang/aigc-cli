package decision

import (
	"encoding/json"
	"fmt"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
)

func buildDecisionCurl(req *provider.DecisionRequest, p *provider.EffectiveProvider) string {
	body, err := json.Marshal(req)
	if err != nil {
		body = []byte("{}")
	}
	cmd := fmt.Sprintf("curl -X POST %s \\\n", provider.SystemOneEndpoint(p.BaseURL))
	if p.APIKey != "" {
		cmd += fmt.Sprintf("  -H \"Authorization: Bearer %s\" \\\n", service.MaskKey(p.APIKey))
	}
	cmd += "  -H \"Content-Type: application/json\" \\\n"
	cmd += fmt.Sprintf("  -d '%s'", string(body))
	return cmd
}

func printJSONResponse(resp *provider.DecisionResponse) error {
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

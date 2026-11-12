package decision

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/reqbuild"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

func buildDecisionCurl(req *provider.DecisionRequest, p *provider.EffectiveProvider) string {
	plan := &reqbuild.Plan{Method: http.MethodPost, URL: provider.SystemOneEndpoint(p.BaseURL), Body: req}
	curl, err := plan.RenderCurlsErr(p.APIKey)
	if err != nil {
		return fmt.Sprintf("# failed to render preview: %v", err)
	}
	return curl
}

func printJSONResponse(resp *provider.DecisionResponse) error {
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	ui.Println(string(data))
	return nil
}

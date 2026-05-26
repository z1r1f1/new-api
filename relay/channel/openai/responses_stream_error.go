package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
)

func newResponsesStreamAPIError(streamResp dto.ResponsesStreamResponse, data string) *types.NewAPIError {
	return types.NewOpenAIError(
		errors.New(formatResponsesStreamError(streamResp, data)),
		types.ErrorCodeBadResponse,
		http.StatusInternalServerError,
	)
}

func formatResponsesStreamError(streamResp dto.ResponsesStreamResponse, data string) string {
	eventType := strings.TrimSpace(streamResp.Type)
	if eventType == "" {
		eventType = "unknown"
	}
	base := fmt.Sprintf("responses stream error: %s", eventType)

	oaiErr, ok := extractResponsesStreamOpenAIError(streamResp, data)
	if !ok {
		return base
	}
	detail := formatResponsesStreamOpenAIErrorDetail(oaiErr)
	if detail == "" {
		return base
	}
	return base + ": " + detail
}

func extractResponsesStreamOpenAIError(streamResp dto.ResponsesStreamResponse, data string) (*types.OpenAIError, bool) {
	if streamResp.Response != nil {
		if oaiErr := streamResp.Response.GetOpenAIError(); responsesStreamOpenAIErrorHasDetail(oaiErr) {
			return oaiErr, true
		}
	}

	var obj map[string]json.RawMessage
	if strings.TrimSpace(data) == "" || common.Unmarshal(common.StringToByteSlice(data), &obj) != nil {
		return nil, false
	}

	if raw, ok := obj["error"]; ok {
		if oaiErr, ok := parseResponsesStreamOpenAIError(raw); ok {
			return oaiErr, true
		}
	}

	rawResponse, ok := obj["response"]
	if !ok {
		return nil, false
	}
	var responseObj map[string]json.RawMessage
	if common.Unmarshal(rawResponse, &responseObj) != nil {
		return nil, false
	}
	if raw, ok := responseObj["error"]; ok {
		if oaiErr, ok := parseResponsesStreamOpenAIError(raw); ok {
			return oaiErr, true
		}
	}
	return nil, false
}

func parseResponsesStreamOpenAIError(raw json.RawMessage) (*types.OpenAIError, bool) {
	var value any
	if common.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	oaiErr := dto.GetOpenAIError(value)
	if !responsesStreamOpenAIErrorHasDetail(oaiErr) {
		return nil, false
	}
	return oaiErr, true
}

func responsesStreamOpenAIErrorHasDetail(oaiErr *types.OpenAIError) bool {
	if oaiErr == nil {
		return false
	}
	return strings.TrimSpace(oaiErr.Message) != "" ||
		strings.TrimSpace(oaiErr.Type) != "" ||
		strings.TrimSpace(oaiErr.Param) != "" ||
		strings.TrimSpace(common.Interface2String(oaiErr.Code)) != "" ||
		len(oaiErr.Metadata) > 0
}

func formatResponsesStreamOpenAIErrorDetail(oaiErr *types.OpenAIError) string {
	if oaiErr == nil {
		return ""
	}
	message := strings.TrimSpace(oaiErr.Message)
	if message != "" {
		message = common.MaskSensitiveInfo(message)
	}

	attrs := make([]string, 0, 3)
	if code := strings.TrimSpace(common.Interface2String(oaiErr.Code)); code != "" {
		attrs = append(attrs, "code="+code)
	}
	if typ := strings.TrimSpace(oaiErr.Type); typ != "" {
		attrs = append(attrs, "type="+typ)
	}
	if param := strings.TrimSpace(oaiErr.Param); param != "" {
		attrs = append(attrs, "param="+common.MaskSensitiveInfo(param))
	}

	if message == "" {
		return strings.Join(attrs, ", ")
	}
	if len(attrs) == 0 {
		return message
	}
	return fmt.Sprintf("%s (%s)", message, strings.Join(attrs, ", "))
}

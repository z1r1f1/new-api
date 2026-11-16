package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOaiResponsesHandlerNormalizesNilOutputForSDKClients(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_test","object":"response","status":"completed","model":"gpt-5.5","output":null,"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`,
		)),
	}

	usage, err := OaiResponsesHandler(c, &relaycommon.RelayInfo{}, resp)
	if err != nil {
		t.Fatalf("OaiResponsesHandler returned error: %v", err)
	}
	if usage == nil || usage.TotalTokens != 2 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"output":[]`) {
		t.Fatalf("expected response output to be normalized to an empty array, got:\n%s", body)
	}
	if strings.Contains(body, `"output":null`) {
		t.Fatalf("expected response output not to be null, got:\n%s", body)
	}
}

func TestOaiResponsesStreamHandlerNormalizesCompletedOutputForSDKClients(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = originalStreamingTimeout })
	for _, eventType := range []string{"response.completed", "response.done"} {
		for _, output := range []string{"", `,"output":null`} {
			t.Run(eventType+output, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

				body := `data: {"type":"` + eventType + `","response":{"id":"resp_test","object":"response","status":"completed","model":"gpt-5.5","usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}` + output + "}}\n\n"
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}
				info := &relaycommon.RelayInfo{IsStream: true}

				usage, err := OaiResponsesStreamHandler(c, info, resp)
				require.Nil(t, err)
				require.NotNil(t, usage)
				assert.Equal(t, 12, usage.TotalTokens)
				assert.Contains(t, recorder.Body.String(), `"output":[]`)
				assert.NotContains(t, recorder.Body.String(), `"output":null`)
			})
		}
	}
}

func TestOaiResponsesStreamHandlerRetainsAcceptedFailureUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = originalStreamingTimeout })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_test","status":"in_progress"}}`,
		`data: {"type":"response.failed","response":{"id":"resp_test","status":"failed","usage":{"input_tokens":7,"output_tokens":5,"total_tokens":12},"error":{"message":"synthetic accepted failure","code":"upstream_error"}}}`,
		``,
	}, "\n\n")
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	info := &relaycommon.RelayInfo{IsStream: true}

	usage, err := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, 7, usage.PromptTokens)
	assert.Equal(t, 5, usage.CompletionTokens)
	assert.Equal(t, "failed", info.StreamStatus.ResponseOutcome())
	assert.Contains(t, recorder.Body.String(), "event: response.failed")
}

func TestOaiResponsesStreamHandlerIncludesResponseFailedDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = originalStreamingTimeout
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://private.example.com")
	common.SetContextKey(c, constant.ContextKeyUpstreamRequestOrigin, "https://route.example.net")

	body := strings.Join([]string{
		`data: {"type":"response.failed","response":{"status":"failed","error":{"message":"Upstream rejected the tool call at https://private.example.com/v1/responses; lookup route.example.net; see https://docs.example.org/errors","code":"tool_call_invalid","param":"https://route.example.net/private?key=secret"}}}`,
		``,
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	info := &relaycommon.RelayInfo{IsStream: true}

	_, err := OaiResponsesStreamHandler(c, info, resp)
	require.Error(t, err)
	errText := err.Error()
	for _, want := range []string{"response.failed", "Upstream rejected the tool call", "tool_call_invalid", "https://***.com/***/***", "lookup ***.***.net", "param=https://***.net/***?key=***", "https://docs.example.org/errors"} {
		assert.Contains(t, errText, want)
	}
	for _, forbidden := range []string{"private.example.com", "route.example.net", "secret"} {
		assert.NotContains(t, errText, forbidden)
	}
	require.NotNil(t, info.StreamStatus)
	if info.StreamStatus.EndError != nil {
		assert.Equal(t, errText, info.StreamStatus.EndError.Error())
	}
	require.NotEmpty(t, info.StreamStatus.Errors)
	for _, diagnostic := range info.StreamStatus.Errors {
		assert.NotContains(t, diagnostic.Message, "private.example.com")
		assert.NotContains(t, diagnostic.Message, "route.example.net")
	}
}

func TestResponsesToolSchemasPreserveRawJSON(t *testing.T) {
	type schemaCase struct {
		name  string
		input string
		want  string
	}
	tests := []schemaCase{
		{
			name:  "flat schema preserves precision order and explicit zero values",
			input: `[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"z":{"const":9007199254740993},"a":{"const":0},"flag":{"const":false}}},"x-vendor":{"z":1e+30, "a":false,"n":null}}]`,
			want:  `[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"z":{"const":9007199254740993},"a":{"const":0},"flag":{"const":false}},"additionalProperties":false},"x-vendor":{"z":1e+30, "a":false,"n":null}}]`,
		},
		{
			name:  "nested Read flattens without changing opaque tools or required values",
			input: `[{"type":"custom","format":{"z":9007199254740993, "a":0}},null,{"type":"function","function":{"name":" Read ","description":" reader ","parameters":{"type":"object","properties":{"z":{"const":9007199254740993},"pages":{"type":"string"},"a":{"const":0}},"required":["pages","z","pages",9007199254740993,false,"a"]}},"strict":true,"x-vendor":{"z":0,"a":false}}]`,
			want:  `[{"type":"custom","format":{"z":9007199254740993, "a":0}},null,{"type":"function","strict":true,"x-vendor":{"z":0,"a":false},"name":"Read","description":" reader ","parameters":{"type":"object","properties":{"z":{"const":9007199254740993},"a":{"const":0}},"required":["z",9007199254740993,false,"a"],"additionalProperties":false}}]`,
		},
		{
			name:  "flat members override legacy nested members",
			input: `[{"type":"function","function":{"name":"ignored","description":"ignored","parameters":{"properties":{"ignored":{}}}},"name":"flat","description":"kept","parameters":{"properties":{"z":{"const":9007199254740993},"a":{"const":0}},"additionalProperties":true}}]`,
			want:  `[{"type":"function","name":"flat","description":"kept","parameters":{"properties":{"z":{"const":9007199254740993},"a":{"const":0}},"additionalProperties":true}}]`,
		},
		{
			name:  "recursive closure preserves escaped names and opaque schema members",
			input: `[{"type":"function","parameters":{"properties":{"n.est":{"items":[{"properties":{"z":{"const":9007199254740993},"a":{"const":0}}}]},"nullable":null},"anyOf":[{"properties":{}}],"oneOf":[{"items":{"properties":{}}}],"allOf":[{"properties":{},"additionalProperties":true}],"required":"invalid","ignored":null,"additionalProperties":null,"x-vendor":{"z":9007199254740993, "a":0}}}]`,
			want:  `[{"type":"function","parameters":{"properties":{"n.est":{"items":[{"properties":{"z":{"const":9007199254740993},"a":{"const":0}},"additionalProperties":false}]},"nullable":null},"anyOf":[{"properties":{},"additionalProperties":false}],"oneOf":[{"items":{"properties":{},"additionalProperties":false}}],"allOf":[{"properties":{},"additionalProperties":true}],"x-vendor":{"z":9007199254740993, "a":0},"additionalProperties":false}}]`,
		},
		{
			name:  "Read drops an empty required list after removing pages",
			input: `[{"type":"function","name":" rEaD ","parameters":{"properties":{"pages":{"type":"string"}},"required":["pages","pages"]}}]`,
			want:  `[{"type":"function","name":" rEaD ","parameters":{"properties":{},"additionalProperties":false}}]`,
		},
	}
	for _, parameters := range []string{"", "null", "false", "0", `"invalid"`, "[]", "{ }"} {
		input := `[{"type":"function"`
		if parameters != "" {
			input += `,"parameters":` + parameters
		}
		tests = append(tests, schemaCase{
			name:  "default parameters " + parameters,
			input: input + `}]`,
			want:  `[{"type":"function","parameters":{"type":"object","properties":{},"additionalProperties":false}}]`,
		})
	}
	for _, input := range []string{
		"", "null", `{ "type":"function" }`,
		`[{"type":"function"},1]`, `[{"type":"function"},]`,
		`[null,{"type":"web_search","x-vendor":{"z":9007199254740993, "a":false}}]`,
	} {
		tests = append(tests, schemaCase{name: "unchanged " + input, input: input, want: input})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := dto.OpenAIResponsesRequest{Model: "gpt-4o-mini", Tools: []byte(tt.input)}
			info := &relaycommon.RelayInfo{OriginModelName: request.Model, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}}
			converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, request)
			require.NoError(t, err)
			normalized, ok := converted.(dto.OpenAIResponsesRequest)
			require.True(t, ok)
			assert.Equal(t, tt.want, string(normalized.Tools))
			assert.Equal(t, tt.input, string(request.Tools), "normalization must not mutate the retry input")
			assert.Equal(t, tt.want, string(normalizeResponsesToolSchemas(normalized.Tools)), "normalization must be idempotent")
		})
	}
}

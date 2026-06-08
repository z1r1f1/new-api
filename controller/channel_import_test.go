package controller

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func buildTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buf := bytes.NewBuffer(nil)
	zw := zip.NewWriter(buf)
	for name, content := range files {
		writer, err := zw.Create(name)
		require.NoError(t, err)
		_, err = writer.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func TestParseChannelCredentialJSONDocumentAcceptsSingleObjectDefaultingToCodex(t *testing.T) {
	credentials, err := parseChannelCredentialJSONDocument("one.json", []byte(`{
		"access_token":"token-a",
		"account_id":"account-a",
		"email":"a@example.com"
	}`))

	require.NoError(t, err)
	require.Len(t, credentials, 1)
	require.Equal(t, constant.ChannelTypeCodex, credentials[0].Type)
	require.Equal(t, "a@example.com", credentials[0].Name)
	require.Contains(t, credentials[0].Key, `"access_token":"token-a"`)
	require.Contains(t, credentials[0].Key, `"account_id":"account-a"`)
}

func TestParseChannelCredentialJSONDocumentAcceptsArrayAndStringType(t *testing.T) {
	credentials, err := parseChannelCredentialJSONDocument("many.json", []byte(`[
		{"access_token":"token-a","account_id":"account-a","email":"a@example.com","type":"codex"},
		{"access_token":"token-b","account_id":"account-b","email":"b@example.com","channel_type":"58"}
	]`))

	require.NoError(t, err)
	require.Len(t, credentials, 2)
	require.Equal(t, constant.ChannelTypeCodex, credentials[0].Type)
	require.Equal(t, constant.ChannelTypeChatGPTImage, credentials[1].Type)
	require.Equal(t, "b@example.com", credentials[1].Name)
}

func TestParseChannelCredentialZipReadsJSONFilesOnly(t *testing.T) {
	zipBytes := buildTestZip(t, map[string]string{
		"nested/a.json": `{"access_token":"token-a","account_id":"account-a","email":"a@example.com"}`,
		"b.txt":         `not-json`,
		"c.json":        `[{"access_token":"token-c","account_id":"account-c","email":"c@example.com"}]`,
	})

	credentials, err := parseChannelCredentialZip("credentials.zip", zipBytes)

	require.NoError(t, err)
	require.Len(t, credentials, 2)
	require.ElementsMatch(t, []string{"a@example.com", "c@example.com"}, []string{credentials[0].Name, credentials[1].Name})
}

func TestBuildChannelsFromCredentialImportsAppliesTemplateAndCredentialOverrides(t *testing.T) {
	template := &model.Channel{
		Type:      constant.ChannelTypeOpenAI,
		Name:      "manual-name-ignored",
		Models:    "gpt-5.5",
		Group:     "svip",
		TestModel: common.GetPointer("gpt-5.5"),
		Status:    1,
		Setting:   common.GetPointer(`{"proxy":"http://127.0.0.1:7890"}`),
	}
	credentials := []channelCredentialImport{
		{Type: constant.ChannelTypeCodex, Name: "a@example.com", Key: `{"access_token":"token-a","account_id":"account-a","email":"a@example.com"}`},
		{Type: constant.ChannelTypeChatGPTImage, Name: "b@example.com", Key: `{"access_token":"token-b","email":"b@example.com"}`},
	}

	channels, err := buildChannelsFromCredentialImports(template, credentials)

	require.NoError(t, err)
	require.Len(t, channels, 2)
	require.Equal(t, constant.ChannelTypeCodex, channels[0].Type)
	require.Equal(t, "a@example.com", channels[0].Name)
	require.Equal(t, "svip", channels[0].Group)
	require.Equal(t, "gpt-5.5", channels[0].Models)
	require.Equal(t, constant.ChannelTypeChatGPTImage, channels[1].Type)
	require.Equal(t, "b@example.com", channels[1].Name)
}

func TestBuildChannelsFromCredentialImportsRejectsInvalidCodexCredential(t *testing.T) {
	template := &model.Channel{Models: "gpt-5.5", Group: "svip", Status: 1}
	credentials := []channelCredentialImport{{
		Type: constant.ChannelTypeCodex,
		Name: "broken@example.com",
		Key:  `{"access_token":"token-only"}`,
	}}

	_, err := buildChannelsFromCredentialImports(template, credentials)

	require.Error(t, err)
	require.Contains(t, err.Error(), "account_id")
}

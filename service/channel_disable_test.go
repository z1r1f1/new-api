package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestShouldDisableChannelSkipsRequestTimeout(t *testing.T) {
	oldEnabled := common.AutomaticDisableChannelEnabled
	oldRanges := operation_setting.AutomaticDisableStatusCodeRanges
	oldKeywords := operation_setting.AutomaticDisableKeywords
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = oldEnabled
		operation_setting.AutomaticDisableStatusCodeRanges = oldRanges
		operation_setting.AutomaticDisableKeywords = oldKeywords
	})

	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 408, End: 408}}
	operation_setting.AutomaticDisableKeywords = []string{"timeout", "response_time_exceeded"}

	channelTimeoutErr := types.NewOpenAIError(
		errors.New("timeout while waiting for upstream"),
		types.ErrorCodeChannelResponseTimeExceeded,
		http.StatusRequestTimeout,
	)
	require.False(t, ShouldDisableChannel(channelTimeoutErr))

	upstream408Err := types.NewOpenAIError(
		errors.New("upstream returned request timeout"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusRequestTimeout,
	)
	require.False(t, ShouldDisableChannel(upstream408Err))
}

func TestShouldDisableChannelStillDisablesConfiguredNon408Errors(t *testing.T) {
	oldEnabled := common.AutomaticDisableChannelEnabled
	oldRanges := operation_setting.AutomaticDisableStatusCodeRanges
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = oldEnabled
		operation_setting.AutomaticDisableStatusCodeRanges = oldRanges
	})

	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 401, End: 401}}

	err := types.NewOpenAIError(
		errors.New("invalid api key"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusUnauthorized,
	)
	require.True(t, ShouldDisableChannel(err))
}

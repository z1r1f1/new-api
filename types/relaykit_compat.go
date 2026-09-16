package types

import relaykittypes "github.com/QuantumNous/new-api/relaykit/types"

type ErrorCode = relaykittypes.ErrorCode
type OpenAIError = relaykittypes.OpenAIError
type NewAPIError = relaykittypes.NewAPIError
type NewAPIErrorOptions = relaykittypes.NewAPIErrorOptions
type RelayFormat = relaykittypes.RelayFormat

const ErrorCodeBadResponse = relaykittypes.ErrorCodeBadResponse
const ErrorCodeBadResponseStatusCode = relaykittypes.ErrorCodeBadResponseStatusCode
const ErrorCodeReadResponseBodyFailed = relaykittypes.ErrorCodeReadResponseBodyFailed
const ErrorCodeBadResponseBody = relaykittypes.ErrorCodeBadResponseBody
const ErrorCodeInvalidRequest = relaykittypes.ErrorCodeInvalidRequest
const ErrorCodeChannelResponseTimeExceeded = relaykittypes.ErrorCodeChannelResponseTimeExceeded
const RelayFormatOpenAI = relaykittypes.RelayFormatOpenAI
const RelayFormatClaude = relaykittypes.RelayFormatClaude
const RelayFormatOpenAIResponses = relaykittypes.RelayFormatOpenAIResponses
const RelayFormatOpenAIResponsesCompaction = relaykittypes.RelayFormatOpenAIResponsesCompaction

var NewOpenAIError = relaykittypes.NewOpenAIError
var WithOpenAIError = relaykittypes.WithOpenAIError
var NewError = relaykittypes.NewError

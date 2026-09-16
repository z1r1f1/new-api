package dto

import relaykitdto "github.com/QuantumNous/new-api/relaykit/dto"

// The host package was split into relaykit/dto upstream. These aliases keep
// local host services source-compatible while all values remain the canonical
// relaykit representations.
type GeneralOpenAIRequest = relaykitdto.GeneralOpenAIRequest
type Message = relaykitdto.Message
type ToolCallRequest = relaykitdto.ToolCallRequest
type ToolCallResponse = relaykitdto.ToolCallResponse
type FunctionRequest = relaykitdto.FunctionRequest
type FunctionResponse = relaykitdto.FunctionResponse
type StreamOptions = relaykitdto.StreamOptions
type OpenAIResponsesRequest = relaykitdto.OpenAIResponsesRequest
type OpenAIResponsesResponse = relaykitdto.OpenAIResponsesResponse
type OpenAITextResponse = relaykitdto.OpenAITextResponse
type OpenAITextResponseChoice = relaykitdto.OpenAITextResponseChoice
type ChatCompletionsStreamResponse = relaykitdto.ChatCompletionsStreamResponse
type ChatCompletionsStreamResponseChoice = relaykitdto.ChatCompletionsStreamResponseChoice
type ChatCompletionsStreamResponseChoiceDelta = relaykitdto.ChatCompletionsStreamResponseChoiceDelta
type ResponsesStreamResponse = relaykitdto.ResponsesStreamResponse
type ResponsesOutput = relaykitdto.ResponsesOutput
type ResponsesOutputContent = relaykitdto.ResponsesOutputContent
type InputTokenDetails = relaykitdto.InputTokenDetails
type OpenAIResponsesCompactionRequest = relaykitdto.OpenAIResponsesCompactionRequest
type OpenAIResponsesCompactionResponse = relaykitdto.OpenAIResponsesCompactionResponse
type Reasoning = relaykitdto.Reasoning
type ResponseFormat = relaykitdto.ResponseFormat
type MessageImageUrl = relaykitdto.MessageImageUrl
type ImageResponse = relaykitdto.ImageResponse
type ImageRequest = relaykitdto.ImageRequest
type ImageData = relaykitdto.ImageData
type GeminiChatRequest = relaykitdto.GeminiChatRequest
type ClaudeRequest = relaykitdto.ClaudeRequest
type ClaudeMessage = relaykitdto.ClaudeMessage
type ClaudeResponse = relaykitdto.ClaudeResponse
type AudioRequest = relaykitdto.AudioRequest
type RerankRequest = relaykitdto.RerankRequest
type EmbeddingRequest = relaykitdto.EmbeddingRequest
type Usage = relaykitdto.Usage

const ContentTypeText = relaykitdto.ContentTypeText
const ResponsesOutputTypeItemAdded = relaykitdto.ResponsesOutputTypeItemAdded
const ResponsesOutputTypeItemDone = relaykitdto.ResponsesOutputTypeItemDone
const ContentTypeImageURL = relaykitdto.ContentTypeImageURL

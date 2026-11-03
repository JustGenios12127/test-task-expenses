package groq

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"test-task-expenses/backend/internal/domain/chat"
)

var errNoChoices = errors.New("empty choices")

type Client struct {
	api   openai.Client
	model string
}

func New(apiKey, model, baseURL string, opts ...option.RequestOption) *Client {
	opts = append([]option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
		option.WithRequestTimeout(30 * time.Second),
		option.WithMaxRetries(2),
	}, opts...)
	return &Client{api: openai.NewClient(opts...), model: model}
}

func (c *Client) Complete(ctx context.Context, system string, messages []chat.Message, tools []chat.Tool) (chat.Message, error) {
	resp, err := c.api.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:       c.model,
		Messages:    toMessages(system, messages),
		Tools:       toTools(tools),
		Temperature: openai.Float(0),
	})
	if err != nil {
		return chat.Message{}, fmt.Errorf("llm completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return chat.Message{}, fmt.Errorf("llm completion: %w", errNoChoices)
	}

	msg := resp.Choices[0].Message
	answer := chat.Message{Role: chat.RoleAssistant, Text: msg.Content}
	for _, call := range msg.ToolCalls {
		if call.Type != "function" {
			continue
		}
		answer.ToolCalls = append(answer.ToolCalls, chat.ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: call.Function.Arguments,
		})
	}
	return answer, nil
}

func toMessages(system string, messages []chat.Message) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages)+1)
	out = append(out, openai.SystemMessage(system))
	for _, msg := range messages {
		switch msg.Role {
		case chat.RoleUser:
			out = append(out, openai.UserMessage(msg.Text))
		case chat.RoleTool:
			out = append(out, openai.ToolMessage(msg.Text, msg.ToolCallID))
		case chat.RoleAssistant:
			out = append(out, assistantMessage(msg))
		}
	}
	return out
}

func assistantMessage(msg chat.Message) openai.ChatCompletionMessageParamUnion {
	param := openai.ChatCompletionAssistantMessageParam{}
	if msg.Text != "" {
		param.Content.OfString = openai.String(msg.Text)
	}
	for _, call := range msg.ToolCalls {
		param.ToolCalls = append(param.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: call.ID,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name:      call.Name,
					Arguments: call.Arguments,
				},
			},
		})
	}
	return openai.ChatCompletionMessageParamUnion{OfAssistant: &param}
}

func toTools(tools []chat.Tool) []openai.ChatCompletionToolUnionParam {
	out := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		out = append(out, openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        tool.Name,
			Description: openai.String(tool.Description),
			Parameters:  openai.FunctionParameters(tool.Parameters),
		}))
	}
	return out
}

package apiserver

import (
        "bufio"
        "context"
        "encoding/json"
        "io"
        "strings"
        "time"
        "github.com/google/uuid"
	"monica-proxy/internal/middleware"
	"monica-proxy/internal/monica"
	"monica-proxy/internal/types"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/sashabaranov/go-openai"
)

// RegisterRoutes 注册 Echo 路由
func RegisterRoutes(e *echo.Echo) {
	// 添加Bearer Token认证中间件
	e.Use(middleware.BearerAuth())

	// ChatGPT 风格的请求转发到 /v1/chat/completions
	e.POST("/v1/chat/completions", handleChatCompletion)
	// 获取支持的模型列表
	e.GET("/v1/models", handleListModels)
}

// handleChatCompletion 接收 ChatGPT 形式的对话请求并转发给 Monica
func handleChatCompletion(c echo.Context) error {
	var req openai.ChatCompletionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Invalid request payload",
		})
	}

	// 检查请求是否包含消息
	if len(req.Messages) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "No messages found",
		})
	}
	// 记录原始的stream设置
        originalStream := req.Stream
       // 强制设置为true以获取SSE流
        req.Stream = true

	// marshalIndent, err := json.MarshalIndent(req, "", "  ")
	// if err != nil {
	// 	return err
	// }
	// log.Printf("Received completion request: \n%s\n", marshalIndent)
	// 将 ChatGPTRequest 转换为 MonicaRequest
	monicaReq, err := types.ChatGPTToMonica(req)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": err.Error(),
		})
	}

	// 调用 Monica 并获取 SSE Stream
	stream, err := monica.SendMonicaRequest(c.Request().Context(), monicaReq)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": err.Error(),
		})
	}
	// Resty 不会自动关闭 Body，需要我们自己来处理
	defer stream.RawBody().Close()
       // 根据原始stream设置决定处理方式
       if !originalStream {
          return handleNonStreamingResponse(c, req, stream.RawBody())
       }

	// 这里直接用流式方式把 SSE 数据返回
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Transfer-Encoding", "chunked")
	c.Response().WriteHeader(http.StatusOK)

	// 将 Monica 的 SSE 数据逐行读出，再以 SSE 格式返回给调用方
	if err := monica.StreamMonicaSSEToClient(req.Model, c.Response().Writer, stream.RawBody()); err != nil {
		return err
	}

	return nil
}

// handleListModels 返回支持的模型列表
func handleListModels(c echo.Context) error {
	models := types.GetSupportedModels()
	return c.JSON(http.StatusOK, models)
}

// 更新Event结构定义
type MonicaSSEEvent struct {
    ID                string    `json:"id"`
    Object            string    `json:"object"`
    Created           int64     `json:"created"`
    Model             string    `json:"model"`
    SystemFingerprint string    `json:"system_fingerprint"`
    Choices           []struct {
        Index        int `json:"index"`
        Delta       struct {
            Content string `json:"content"`
            Role    string `json:"role"`
        } `json:"delta"`
        FinishReason string `json:"finish_reason"`
    } `json:"choices"`
}

func handleNonStreamingResponse(c echo.Context, req openai.ChatCompletionRequest, body io.ReadCloser) error {
    ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
    defer cancel()

    var fullContent strings.Builder
    var firstEvent *MonicaSSEEvent // 保存第一个事件用于获取元数据
    scanner := bufio.NewScanner(body)

    for scanner.Scan() {
        select {
        case <-ctx.Done():
            return c.JSON(http.StatusGatewayTimeout, map[string]interface{}{
                "error": "request timeout",
            })
        default:
            line := scanner.Text()
            if strings.HasPrefix(line, "data: ") {
                data := line[6:]
                if data == "[DONE]" {
                    continue
                }

                var event MonicaSSEEvent
                if err := json.Unmarshal([]byte(data), &event); err != nil {
                    continue
                }

                // 保存第一个事件的元数据
                if firstEvent == nil {
                    firstEvent = &event
                }

                if len(event.Choices) > 0 && event.Choices[0].Delta.Content != "" {
                    fullContent.WriteString(event.Choices[0].Delta.Content)
                }
            }
        }
    }

    if err := scanner.Err(); err != nil {
        return c.JSON(http.StatusInternalServerError, map[string]interface{}{
            "error": "Error reading response: " + err.Error(),
        })
    }

    // 使用第一个事件的元数据构造响应
    response := openai.ChatCompletionResponse{
        ID:                firstEvent.ID,
        Object:           "chat.completion",
        Created:          firstEvent.Created,
        Model:            firstEvent.Model,
        SystemFingerprint: firstEvent.SystemFingerprint,
        Choices: []openai.ChatCompletionChoice{
            {
                Index: 0,
                Message: openai.ChatCompletionMessage{
                    Role:    "assistant", // 使用固定的assistant角色
                    Content: fullContent.String(),
                },
                FinishReason: "stop",
            },
        },
        Usage: openai.Usage{
            PromptTokens:     0,
            CompletionTokens: 0,
            TotalTokens:      0,
        },
    }

    return c.JSON(http.StatusOK, response)
}



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

func handleNonStreamingResponse(c echo.Context, req openai.ChatCompletionRequest, body io.Reader) error {
    reader := bufio.NewReader(body)
    var sb strings.Builder
    
    // 用于解析SSE数据
    var streamResp types.ChatCompletionStreamResponse
    
    created := time.Now().Unix()
    chatId := uuid.New().String()
    
    for {
        line, err := reader.ReadString('\n')
        if err != nil {
            if err == io.EOF {
                break
            }
            return err
        }

        // 跳过非data行
        if !strings.HasPrefix(line, "data: ") {
            continue
        }

        // 去掉data: 前缀
        jsonStr := strings.TrimPrefix(line, "data: ")
        jsonStr = strings.TrimSpace(jsonStr)
        
        // 检查是否结束
        if jsonStr == "[DONE]" {
            break
        }

        // 解析SSE JSON
        if err := json.Unmarshal([]byte(jsonStr), &streamResp); err != nil {
            continue
        }

        // 获取content并拼接
        if len(streamResp.Choices) > 0 {
            content := streamResp.Choices[0].Delta.Content
            // 处理thinking标记
            if content == "<think>" {
                continue
            }
            if strings.HasPrefix(content, "</think>") {
                content = strings.TrimPrefix(content, "</think>")
            }
            sb.WriteString(content)
        }
    }

    // 构造非流式响应
    response := &openai.ChatCompletionResponse{
        ID:      "chatcmpl-" + chatId,
        Object:  "chat.completion",
        Created: created,
        Model:   req.Model,
        Choices: []openai.ChatCompletionChoice{
            {
                Index: 0,
                Message: openai.ChatCompletionMessage{
                    Role:    openai.ChatMessageRoleAssistant,
                    Content: sb.String(),
                },
                FinishReason: openai.FinishReasonStop,
            },
        },
        Usage: openai.Usage{
            // Monica不提供token统计,这里简单置0
            PromptTokens:     0,
            CompletionTokens: 0,
            TotalTokens:      0,
        },
    }

    return c.JSON(http.StatusOK, response)
}



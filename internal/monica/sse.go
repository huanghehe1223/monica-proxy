package monica

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"monica-proxy/internal/types"
	"monica-proxy/internal/utils"
	"net/http"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/sashabaranov/go-openai"
)

const (
	sseObject     = "chat.completion.chunk"
	sseFinish     = "[DONE]"
	flushInterval = 100 * time.Millisecond // 刷新间隔
	bufferSize    = 4096                   // 缓冲区大小
)

// SSEData 用于解析 Monica SSE json
type SSEData struct {
	Text        string      `json:"text"`
	Finished    bool        `json:"finished"`
	AgentStatus AgentStatus `json:"agent_status,omitempty"`
}

type AgentStatus struct {
	UID      string `json:"uid"`
	Type     string `json:"type"`
	Text     string `json:"text"`
	Metadata struct {
		Title           string `json:"title"`
		ReasoningDetail string `json:"reasoning_detail"`
	} `json:"metadata"`
}

var sseDataPool = sync.Pool{
	New: func() interface{} {
		return &SSEData{}
	},
}

// StreamMonicaSSEToClient 将 Monica SSE 转成前端可用的流
func StreamMonicaSSEToClient(model string, w io.Writer, r io.Reader) error {
	reader := bufio.NewReaderSize(r, bufferSize)
	writer := bufio.NewWriterSize(w, bufferSize)
	defer writer.Flush()

	chatId := utils.RandStringUsingMathRand(29)
	now := time.Now().Unix()
	fingerprint := utils.RandStringUsingMathRand(10)

	// 创建一个定时刷新的 ticker
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	// 创建一个 done channel 用于清理
	done := make(chan struct{})
	defer close(done)

	// 启动一个 goroutine 定期刷新缓冲区
	go func() {
		for {
			select {
			case <-ticker.C:
				if f, ok := w.(http.Flusher); ok {
					writer.Flush()
					f.Flush()
				}
			case <-done:
				return
			}
		}
	}()

	var thinkFlag bool
	for {
		line, err := reader.ReadString('\n')
		fmt.Printf("Stream SSE raw line: %s\n", line)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read error: %w", err)
		}

		// Monica SSE 的行前缀一般是 "data: "
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		jsonStr := strings.TrimPrefix(line, "data: ")
		if jsonStr == "" {
			continue
		}

		// 从对象池获取 SSEData
		sseObj := sseDataPool.Get().(*SSEData)
		if err := sonic.UnmarshalString(jsonStr, sseObj); err != nil {
			sseDataPool.Put(sseObj)
			// 记录错误但继续处理
			log.Printf("Error unmarshaling SSE data: %v", err)
			continue
		}

		// 将拆分后的文字写回
		var sseMsg types.ChatCompletionStreamResponse
		switch {
		case sseObj.Finished:
			sseMsg = types.ChatCompletionStreamResponse{
				ID:      "chatcmpl-" + chatId,
				Object:  sseObject,
				Created: now,
				Model:   model,
				Choices: []types.ChatCompletionStreamChoice{
					{
						Index: 0,
						Delta: openai.ChatCompletionStreamChoiceDelta{
							Role: openai.ChatMessageRoleAssistant,
						},
						FinishReason: openai.FinishReasonStop,
					},
				},
			}
		case sseObj.AgentStatus.Type == "thinking":
			thinkFlag = true
			sseMsg = types.ChatCompletionStreamResponse{
				ID:                "chatcmpl-" + chatId,
				Object:            sseObject,
				SystemFingerprint: fingerprint,
				Created:           now,
				Model:             model,
				Choices: []types.ChatCompletionStreamChoice{
					{
						Index: 0,
						Delta: openai.ChatCompletionStreamChoiceDelta{
							Role:    openai.ChatMessageRoleAssistant,
							Content: `<think>`,
						},
						FinishReason: openai.FinishReasonNull,
					},
				},
			}
		case sseObj.AgentStatus.Type == "thinking_detail_stream":
			sseMsg = types.ChatCompletionStreamResponse{
				ID:                "chatcmpl-" + chatId,
				Object:            sseObject,
				SystemFingerprint: fingerprint,
				Created:           now,
				Model:             model,
				Choices: []types.ChatCompletionStreamChoice{
					{
						Index: 0,
						Delta: openai.ChatCompletionStreamChoiceDelta{
							Role:    openai.ChatMessageRoleAssistant,
							Content: sseObj.AgentStatus.Metadata.ReasoningDetail,
						},
						FinishReason: openai.FinishReasonNull,
					},
				},
			}
		default:
			if thinkFlag {
				sseObj.Text = "</think>" + sseObj.Text
				thinkFlag = false
			}
			sseMsg = types.ChatCompletionStreamResponse{
				ID:                "chatcmpl-" + chatId,
				Object:            sseObject,
				SystemFingerprint: fingerprint,
				Created:           now,
				Model:             model,
				Choices: []types.ChatCompletionStreamChoice{
					{
						Index: 0,
						Delta: openai.ChatCompletionStreamChoiceDelta{
							Role:    openai.ChatMessageRoleAssistant,
							Content: sseObj.Text,
						},
						FinishReason: openai.FinishReasonNull,
					},
				},
			}
		}

		var sb strings.Builder
		sb.WriteString("data: ")
		sendLine, _ := sonic.MarshalString(sseMsg)
		sb.WriteString(sendLine)
		sb.WriteString("\n\n")

		// 写入缓冲区
		if _, err := writer.WriteString(sb.String()); err != nil {
			return fmt.Errorf("write error: %w", err)
		}

		// 如果发现 finished=true，就可以结束
		if sseObj.Finished {
			writer.WriteString(fmt.Sprintf("data: %s\n\n", sseFinish))
			writer.Flush()
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			return nil
		}

		sseObj.AgentStatus.Type = ""
		sseObj.Finished = false
		sseDataPool.Put(sseObj)
	}
}

// handleNonStreamingResponse 处理非流式响应
func HandleNonStreamingResponse(model string, r io.Reader) (*types.ChatCompletionResponse, error) {
    reader := bufio.NewReaderSize(r, bufferSize)
    var contentBuilder strings.Builder
    
    // 生成响应所需的ID等信息
    chatId := utils.RandStringUsingMathRand(29)
    now := time.Now().Unix()
    
    for {
        line, err := reader.ReadString('\n')
	fmt.Printf("Non-Stream SSE raw line: %s\n", line) 
        if err != nil {
            if err == io.EOF {
                break
            }
            return nil, fmt.Errorf("read error: %w", err)
        }

        // 跳过非data行
        if !strings.HasPrefix(line, "data: ") {
            continue
        }

        jsonStr := strings.TrimPrefix(line, "data: ")
        if jsonStr == "" {
            continue
        }

        // 解析SSE数据
        var sseObj SSEData
        if err := sonic.UnmarshalString(jsonStr, &sseObj); err != nil {
            // 记录错误但继续处理
            log.Printf("Error unmarshaling SSE data: %v", err)
            continue
        }

            contentBuilder.WriteString(sseObj.Text)


        // 如果发现finished标记，结束处理
        if sseObj.Finished {
            break
        }
    }

    // 构造完整的响应
    response := &types.ChatCompletionResponse{
        ID:      "chatcmpl-" + chatId,
        Object:  "chat.completion",
        Created: now,
        Model:   model,
        Choices: []types.ChatCompletionChoice{
            {
                Index: 0,
                Message: types.ChatCompletionMessage{
                    Role:    "assistant",
                    Content: contentBuilder.String(),
                },
                FinishReason: "stop",
            },
        },
        Usage: types.ChatCompletionUsage{
            // 这里可以根据需要设置token使用情况
            PromptTokens:     0,
            CompletionTokens: 0,
            TotalTokens:      0,
        },
    }

    return response, nil
}



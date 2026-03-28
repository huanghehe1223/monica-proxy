package monica

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	
	"monica-proxy/internal/config"
	"monica-proxy/internal/errors"
	"monica-proxy/internal/logger"
	"monica-proxy/internal/types"
	"monica-proxy/internal/utils"

	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"
)

type logReadCloser struct {
	io.Reader
	origin io.ReadCloser
	file   *os.File
}

func (m *logReadCloser) Close() error {
	m.file.Close()
	return m.origin.Close()
}

// SendMonicaRequest 发起对 Monica AI 的请求(使用 resty)
func SendMonicaRequest(ctx context.Context, cfg *config.Config, mReq *types.MonicaRequest) (*resty.Response, error) {
	// 构建请求
	req := utils.RestySSEClient.R().
		SetContext(ctx).
		SetHeader("cookie", cfg.Monica.Cookie).
		SetBody(mReq)

	// 记录原始请求到日志文件
	os.MkdirAll("logs", 0755)
	if fReq, err := os.OpenFile("logs/req_normal.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		bodyBytes, _ := json.MarshalIndent(mReq, "", "  ")
		fReq.WriteString("--- [Normal Chat Request] ---\n")
		fReq.WriteString(fmt.Sprintf("URL: %s\n", types.BotChatURL))
		fReq.WriteString(fmt.Sprintf("Headers: %v\n", req.Header))
		fReq.WriteString(fmt.Sprintf("Body:\n%s\n\n", string(bodyBytes)))
		fReq.Close()
	}

	// 发起请求
	resp, err := req.Post(types.BotChatURL)

	if err != nil {
		logger.Error("Monica API请求失败", zap.Error(err))
		return nil, errors.NewRequestFailedError("Monica API调用失败", err)
	}

	// 拦截原始响应并保存到日志文件
	os.MkdirAll("logs", 0755)
	f, fileErr := os.OpenFile("logs/log.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if fileErr == nil && resp.RawResponse != nil && resp.RawResponse.Body != nil {
		f.WriteString("\n\n--- [Normal Chat Session] ---\n")
		originalBody := resp.RawResponse.Body
		resp.RawResponse.Body = &logReadCloser{
			Reader: io.TeeReader(originalBody, f),
			origin: originalBody,
			file:   f,
		}
	}

	// 如果需要在这里做更多判断，可自行补充
	return resp, nil
}

// SendCustomBotRequest 发送custom bot请求
func SendCustomBotRequest(ctx context.Context, cfg *config.Config, customBotReq *types.CustomBotRequest) (*resty.Response, error) {
	// 构建请求
	req := utils.RestySSEClient.R().
		SetContext(ctx).
		SetHeader("cookie", cfg.Monica.Cookie).
		SetBody(customBotReq)

	// 记录原始请求到日志文件
	os.MkdirAll("logs", 0755)
	if fReq, err := os.OpenFile("logs/req_custom.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		bodyBytes, _ := json.MarshalIndent(customBotReq, "", "  ")
		fReq.WriteString("--- [Custom Bot Chat Request] ---\n")
		fReq.WriteString(fmt.Sprintf("URL: %s\n", types.CustomBotChatURL))
		fReq.WriteString(fmt.Sprintf("Headers: %v\n", req.Header))
		fReq.WriteString(fmt.Sprintf("Body:\n%s\n\n", string(bodyBytes)))
		fReq.Close()
	}

	// 发起请求
	resp, err := req.Post(types.CustomBotChatURL)

	if err != nil {
		logger.Error("Custom Bot API请求失败", zap.Error(err))
		return nil, errors.NewRequestFailedError("Custom Bot API调用失败", err)
	}

	// 拦截原始响应并保存到日志文件
	os.MkdirAll("logs", 0755)
	f, fileErr := os.OpenFile("logs/log_cus.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if fileErr == nil && resp.RawResponse != nil && resp.RawResponse.Body != nil {
		f.WriteString("\n\n--- [Custom Bot Chat Session] ---\n")
		originalBody := resp.RawResponse.Body
		resp.RawResponse.Body = &logReadCloser{
			Reader: io.TeeReader(originalBody, f),
			origin: originalBody,
			file:   f,
		}
	}

	return resp, nil
}

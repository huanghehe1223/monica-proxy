package service

import (
	"context"
	"encoding/json"
	"time"

	"monica-proxy/internal/config"
	"monica-proxy/internal/logger"
	"monica-proxy/internal/types"
	"monica-proxy/internal/utils"

	"go.uber.org/zap"
)

// ModelService 模型服务接口
type ModelService interface {
	// GetSupportedModels 获取支持的模型列表
	GetSupportedModels() []string
	
	// StartModelSync 启动后台协程定时刷新模型列表
	StartModelSync(ctx context.Context)
}

// modelService 模型服务实现
type modelService struct {
	config *config.Config
}

// NewModelService 创建模型服务实例
func NewModelService(cfg *config.Config) ModelService {
	s := &modelService{
		config: cfg,
	}
	
	// 启动时在后台做一次初始化，以及后续定期刷新
	go s.StartModelSync(context.Background())
	
	return s
}

// GetSupportedModels 获取支持的模型列表
func (s *modelService) GetSupportedModels() []string {
	models := types.GetSupportedModels()
	
	logger.Info("获取支持的模型列表",
		zap.Int("model_count", len(models)),
	)
	
	return models
}

// StartModelSync 启动后台协程定时刷新模型列表
func (s *modelService) StartModelSync(ctx context.Context) {
	// 启动立即刷新一次
	s.fetchAndMapDynamicModels(ctx)

	// 每 2 小时刷新一次
	ticker := time.NewTicker(2 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("停止动态模型映射刷新任务")
			return
		case <-ticker.C:
			s.fetchAndMapDynamicModels(ctx)
		}
	}
}

// fetchAndMapDynamicModels 发送请求获取Pined Bot并生成最新映射关系
func (s *modelService) fetchAndMapDynamicModels(ctx context.Context) {
	resp, err := utils.RestyDefaultClient.R().
		SetContext(ctx).
		SetHeader("cookie", s.config.Monica.Cookie).
		SetBody(map[string]any{}).
		Post(types.ListPinedBotURL)

	if err != nil {
		logger.Error("获取动态模型列表请求失败", zap.Error(err))
		return
	}

	bodyBytes := resp.Body()
	if len(bodyBytes) == 0 {
		logger.Error("获取动态模型列表返回体为空",
			zap.Int("status_code", resp.StatusCode()),
		)
		return
	}

	var lists types.ListPinedBotResponse
	if err := json.Unmarshal(bodyBytes, &lists); err != nil {
		logger.Error("解析获取动态模型列表失败",
			zap.Error(err),
			zap.String("response_body", string(bodyBytes)),
		)
		return
	}

	if lists.Code != 0 {
		logger.Error("获取动态模型列表接口返回错误", zap.Int("code", lists.Code), zap.String("msg", lists.Msg))
		return
	}

	newMap := make(map[string]string)
	for _, bot := range lists.Data.PinBots {
		// 仅使用官方模型（Monica Team user_id=98906）构建映射
		if bot.UserID == 98906 && bot.ToolData.UseModel != "" && bot.UID != "" {
			newMap[bot.ToolData.UseModel] = bot.UID
		}
	}

	// 增量更新底层内存映射
	if len(newMap) > 0 {
		types.UpdateDynamicModels(newMap)
		logger.Info("成功刷新动态模型映射表",
			zap.Int("dynamic_model_count", len(newMap)),
			zap.Int("source_pin_bot_count", len(lists.Data.PinBots)),
		)
	}
}

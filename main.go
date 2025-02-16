package main

import (
	"errors"
	"log"
	"monica-proxy/internal/apiserver"
	"monica-proxy/internal/config"
	"net/http"

	"github.com/labstack/echo/v4/middleware"

	"github.com/labstack/echo/v4"
)

func main() {
	// 加载配置
	cfg := config.LoadConfig()
	if cfg.MonicaCookie == "" {
		log.Fatal("MONICA_COOKIE environment variable is required")
	}

	e := echo.New()
    // 配置Echo Logger
    e.Logger.SetLevel(log.DEBUG) // 设置日志级别为DEBUG以显示所有日志
    
    // 配置日志输出格式
    e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
        Format: `{"time":"${time_rfc3339_nano}","level":"${level}","id":"${id}",` +
            `"remote_ip":"${remote_ip}","host":"${host}","method":"${method}","uri":"${uri}",` +
            `"user_agent":"${user_agent}","status":${status},"error":"${error}","latency":${latency},` +
            `"latency_human":"${latency_human}","bytes_in111":${bytes_in},"bytes_out":${bytes_out},` +
            `"message":"${message}"}` + "\n",
        Output: os.Stdout,
    }))

    // 添加自定义日志中间件
    e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            // 将Echo logger实例添加到上下文中
            c.Set("logger", e.Logger)
            return next(c)
        }
    })
	e.Use(middleware.Recover())
	// 注册路由
	apiserver.RegisterRoutes(e)
	// 启动服务
	if err := e.Start("0.0.0.0:8080"); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("start server error: %v", err)
	}
}

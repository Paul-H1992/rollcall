package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"rollcall-pro/internal/ai"
	"rollcall-pro/internal/api"
	"rollcall-pro/internal/db"
	"rollcall-pro/internal/ws"

	"github.com/gin-gonic/gin"
)

var (
	port     = flag.Int("port", 8080, "server port")
	dbPath   = flag.String("db", "./data/rollcall.db", "database path")
	aiKey    = flag.String("ai-key", "", "MiniMax API key")
	dataDir  = flag.String("data", "./data", "data directory")
)

func main() {
	flag.Parse()

	// 创建数据目录
	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// 初始化数据库
	database, err := db.New(*dbPath)
	if err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}
	defer database.Close()
	log.Println("Database initialized")

	// 初始化AI客户端
	aiClient := ai.NewMiniMaxClient(*aiKey)
	log.Println("AI client initialized")

	// 初始化WebSocket Hub
	hub := ws.NewHub()
	go hub.Run()
	log.Println("WebSocket hub started")

	// 初始化API服务器
	server := api.NewServer(database, hub, aiClient)

	// 初始化Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// CORS中间件
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Web路由
	r.Static("/static", "./web/static")
	r.LoadHTMLGlob("./web/*.html")

	// 页面路由
	r.GET("/", func(c *gin.Context) {
		c.Redirect("/teacher", 302)
	})
	r.GET("/teacher", func(c *gin.Context) {
		c.HTML(http.StatusOK, "teacher.html", nil)
	})
	r.GET("/student", func(c *gin.Context) {
		c.HTML(http.StatusOK, "student.html", nil)
	})

	// API路由
	apiGroup := r.Group("/api")
	{
		// 学生管理
		apiGroup.GET("/students", server.GetStudents)
		apiGroup.POST("/students", server.CreateStudent)
		apiGroup.PUT("/students/:id", server.UpdateStudent)
		apiGroup.DELETE("/students/:id", server.DeleteStudent)
		apiGroup.POST("/students/import", server.ImportStudents)
		apiGroup.PUT("/students/:id/status", server.SetStudentStatus)

		// 冷却管理
		apiGroup.GET("/cooldowns", server.GetCooldowns)
		apiGroup.DELETE("/cooldowns/:id", server.RemoveCooldown)

		// 点名
		apiGroup.POST("/call/start", server.StartCall)
		apiGroup.GET("/call/records", server.GetCallRecords)
		apiGroup.GET("/call/state", server.GetCurrentState)
		apiGroup.POST("/call/pause", server.PauseInteraction)
		apiGroup.POST("/call/resume", server.ResumeInteraction)
		apiGroup.POST("/call/skip", server.SkipCurrent)
		apiGroup.POST("/call/end", server.EndInteraction)

		// 题目
		apiGroup.GET("/questions", server.GetQuestions)
		apiGroup.POST("/questions/generate", server.GenerateQuestion)
		apiGroup.POST("/questions", server.SaveQuestion)
		apiGroup.DELETE("/questions/:id", server.DeleteQuestion)

		// 任务
		apiGroup.GET("/tasks", server.GetTasks)
		apiGroup.POST("/tasks", server.PublishTask)
		apiGroup.GET("/tasks/:id/feedbacks", server.GetTaskFeedbacks)
		apiGroup.POST("/tasks/feedback", server.TaskFeedback)

		// 导出
		apiGroup.GET("/export/records", server.ExportRecords)

		// 配置
		apiGroup.GET("/config", server.GetConfig)
		apiGroup.GET("/stages", server.GetStages)
	}

	// WebSocket路由
	r.GET("/ws/teacher", func(c *gin.Context) {
		hub.HandleWebSocket(c, true)
	})
	r.GET("/ws/student", func(c *gin.Context) {
		hub.HandleWebSocket(c, false)
	})

	// 启动服务器
	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Server starting on %s", addr)
	log.Printf("Teacher panel: http://localhost%s/teacher", addr)
	log.Printf("Student panel: http://localhost%s/student", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// 获取可执行文件路径
func getExeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

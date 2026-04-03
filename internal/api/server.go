package api

import (
	"net/http"
	"rollcall-pro/internal/ai"
	"rollcall-pro/internal/db"
	"rollcall-pro/internal/models"
	"rollcall-pro/internal/ws"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Server struct {
	db  *db.DB
	hub *ws.Hub
	ai  *ai.MiniMaxClient
}

func NewServer(database *db.DB, hub *ws.Hub, aiClient *ai.MiniMaxClient) *Server {
	return &Server{db: database, hub: hub, ai: aiClient}
}

// ---- 学生管理 ----

func (s *Server) CreateStudent(c *gin.Context) {
	var req models.CreateStudentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	student := &models.Student{Name: req.Name, StudentNo: req.StudentNo}
	if err := s.db.CreateStudent(student); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, student)
}

func (s *Server) GetStudents(c *gin.Context) {
	students, err := s.db.GetStudents()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, students)
}

func (s *Server) UpdateStudent(c *gin.Context) {
	id := c.Param("id")
	var req models.CreateStudentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	student, err := s.db.GetStudentByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "student not found"})
		return
	}
	student.Name = req.Name
	student.StudentNo = req.StudentNo
	if err := s.db.UpdateStudent(student); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, student)
}

func (s *Server) DeleteStudent(c *gin.Context) {
	id := c.Param("id")
	if err := s.db.DeleteStudent(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (s *Server) ImportStudents(c *gin.Context) {
	var req models.ImportStudentsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.db.BatchCreateStudents(req.Students); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "imported", "count": len(req.Students)})
}

func (s *Server) SetStudentStatus(c *gin.Context) {
	id := c.Param("id")
	status := c.Query("status") // normal/leave/exclude
	if status != "normal" && status != "leave" && status != "exclude" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}
	student, err := s.db.GetStudentByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "student not found"})
		return
	}
	student.Status = status
	if err := s.db.UpdateStudent(student); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, student)
}

// ---- 冷却管理 ----

func (s *Server) GetCooldowns(c *gin.Context) {
	cooldowns, err := s.db.GetCooldowns()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cooldowns)
}

func (s *Server) RemoveCooldown(c *gin.Context) {
	id := c.Param("id")
	// 先获取studentID
	students, _ := s.db.GetStudents()
	var studentID string
	for _, st := range students {
		if st.ID == id {
			studentID = id
			break
		}
	}
	if studentID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "student not found"})
		return
	}
	if err := s.db.RemoveCooldown(studentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "cooldown removed"})
}

// ---- 点名 ----

func (s *Server) StartCall(c *gin.Context) {
	var req models.StartCallReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Mode != "multi_one" && req.Mode != "multi_multi" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be multi_one or multi_multi"})
		return
	}

	// 随机抽取回答者
	answerers, err := s.db.RandomSelectAnswerers(req.AnswererCnt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(answerers) < req.AnswererCnt {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not enough available students"})
		return
	}

	// 创建点名记录
	record := &models.CallRecord{
		Mode:        req.Mode,
		AnswererCnt: req.AnswererCnt,
		AskerCnt:    req.AskerCnt,
	}
	if err := s.db.CreateCallRecord(record); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 保存回答者
	answererIDs := make([]string, len(answerers))
	for i, a := range answerers {
		answererIDs[i] = a.ID
		detail := &models.CallDetail{
			CallRecordID: record.ID,
			StudentID:    a.ID,
			Role:         "answerer",
			Order:        i,
		}
		s.db.CreateCallDetail(detail)
	}

	// 多对一模式：抽取提问者
	askersMap := make(map[int][]string)
	if req.Mode == "multi_multi" {
		for i, a := range answerers {
			// 提问者排除当前回答者
			askers, err := s.db.RandomSelectAskers(req.AskerCnt, []string{a.ID})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			askerIDs := make([]string, len(askers))
			for j, asker := range askers {
				askerIDs[j] = asker.ID
				detail := &models.CallDetail{
					CallRecordID: record.ID,
					StudentID:    asker.ID,
					Role:         "asker",
					Order:        i*req.AskerCnt + j,
				}
				s.db.CreateCallDetail(detail)
			}
			askersMap[i] = askerIDs
		}
	}

	// 更新WebSocket状态
	state := &models.InteractionState{
		Mode:        req.Mode,
		AnswererCnt: req.AnswererCnt,
		AskerCnt:    req.AskerCnt,
		Status:      "running",
		Answerers:   answererIDs,
		AskersMap:   askersMap,
	}
	s.hub.UpdateState(state)

	c.JSON(http.StatusCreated, gin.H{
		"record":     record,
		"answerers":  answerers,
		"askers_map": askersMap,
	})
}

func (s *Server) GetCallRecords(c *gin.Context) {
	records, err := s.db.GetCallRecords()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, records)
}

func (s *Server) GetCurrentState(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "state endpoint"})
}

// ---- 互动控制 ----

func (s *Server) PauseInteraction(c *gin.Context) {
	// 暂停当前互动
	c.JSON(http.StatusOK, gin.H{"message": "paused"})
}

func (s *Server) ResumeInteraction(c *gin.Context) {
	// 继续互动
	c.JSON(http.StatusOK, gin.H{"message": "resumed"})
}

func (s *Server) SkipCurrent(c *gin.Context) {
	// 跳过当前
	c.JSON(http.StatusOK, gin.H{"message": "skipped"})
}

func (s *Server) EndInteraction(c *gin.Context) {
	// 结束互动，并为回答者添加冷却
	// TODO: 实现冷却添加逻辑
	s.hub.UpdateState(&models.InteractionState{Status: "finished"})
	c.JSON(http.StatusOK, gin.H{"message": "ended"})
}

// ---- 题目管理 ----

func (s *Server) GenerateQuestion(c *gin.Context) {
	var req models.GenerateQuestionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	question, err := s.ai.GenerateInterviewQuestion(req.Stage, req.Tags)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, question)
}

func (s *Server) SaveQuestion(c *gin.Context) {
	var question models.Question
	if err := c.ShouldBindJSON(&question); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.db.CreateQuestion(&question); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, question)
}

func (s *Server) GetQuestions(c *gin.Context) {
	questions, err := s.db.GetQuestions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, questions)
}

func (s *Server) DeleteQuestion(c *gin.Context) {
	id := c.Param("id")
	if err := s.db.DeleteQuestion(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ---- 任务管理 ----

func (s *Server) PublishTask(c *gin.Context) {
	var req models.PublishTaskReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	task := &models.Task{
		Content:  req.Content,
		Deadline: req.Deadline,
	}
	if err := s.db.CreateTask(task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.hub.BroadcastTask(task)
	c.JSON(http.StatusCreated, task)
}

func (s *Server) GetTasks(c *gin.Context) {
	tasks, err := s.db.GetTasks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tasks)
}

func (s *Server) TaskFeedback(c *gin.Context) {
	var req models.TaskFeedbackReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	studentID := c.Query("student_id")
	if studentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "student_id required"})
		return
	}

	// 检查是否已反馈
	hasFeedback, _ := s.db.HasTaskFeedback(req.TaskID, studentID)
	if hasFeedback {
		c.JSON(http.StatusOK, gin.H{"message": "already feedback"})
		return
	}

	feedback := &models.TaskFeedback{
		TaskID:    req.TaskID,
		StudentID: studentID,
	}
	if err := s.db.AddTaskFeedback(feedback); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 获取最新反馈统计
	feedbacks, _ := s.db.GetTaskFeedbacks(req.TaskID)
	students, _ := s.db.GetStudents()
	s.hub.BroadcastFeedback(req.TaskID, feedbacks, len(students))

	c.JSON(http.StatusOK, gin.H{"message": "feedback recorded"})
}

func (s *Server) GetTaskFeedbacks(c *gin.Context) {
	taskID := c.Param("id")
	feedbacks, err := s.db.GetTaskFeedbacks(taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	students, _ := s.db.GetStudents()
	c.JSON(http.StatusOK, gin.H{
		"feedbacks": feedbacks,
		"total":     len(students),
	})
}

// ---- 导出 ----

func (s *Server) ExportRecords(c *gin.Context) {
	records, err := s.db.ExportCallRecords()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, records)
}

// ---- 阶段信息 ----

func (s *Server) GetStages(c *gin.Context) {
	c.JSON(http.StatusOK, models.StageMap)
}

// ---- 配置 ----

func (s *Server) GetConfig(c *gin.Context) {
	students, _ := s.db.GetStudents()
	cooldowns, _ := s.db.GetCooldowns()
	questions, _ := s.db.GetQuestions()

	c.JSON(http.StatusOK, gin.H{
		"total_students":  len(students),
		"active_cooldowns": len(cooldowns),
		"total_questions": len(questions),
		"stages":          models.StageMap,
	})
}

// 辅助函数
func getStudentName(students []models.Student, id string) string {
	for _, s := range students {
		if s.ID == id {
			return s.Name
		}
	}
	return id
}

func parseInt(c *gin.Context, key string, defaultVal int) int {
	val, err := strconv.Atoi(c.DefaultQuery(key, strconv.Itoa(defaultVal)))
	if err != nil {
		return defaultVal
	}
	return val
}

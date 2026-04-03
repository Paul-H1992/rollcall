package models

import "time"

// Student 学生模型
type Student struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	StudentNo string    `json:"student_no"` // 学号
	Status    string    `json:"status"`     // normal/leave/exclude
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CallRecord 点名记录
type CallRecord struct {
	ID          string    `json:"id"`
	Mode        string    `json:"mode"`         // multi_one / multi_multi
	AnswererCnt int       `json:"answerer_cnt"` // N
	AskerCnt    int       `json:"asker_cnt"`    // M
	CreatedAt   time.Time `json:"created_at"`
}

// CallDetail 点名明细（回答者和提问者）
type CallDetail struct {
	ID           string    `json:"id"`
	CallRecordID string    `json:"call_record_id"`
	StudentID    string    `json:"student_id"`
	Role         string    `json:"role"` // answerer/asker
	Order        int       `json:"order"`
	CreatedAt    time.Time `json:"created_at"`
}

// Cooldown 冷却记录
type Cooldown struct {
	ID        string    `json:"id"`
	StudentID string    `json:"student_id"`
	StartDate string    `json:"start_date"` // YYYY-MM-DD
	EndDate   string    `json:"end_date"`   // YYYY-MM-DD
	Days      int       `json:"days"`       // 冷却天数
	CreatedAt time.Time `json:"created_at"`
}

// Question 题目
type Question struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Answer    string    `json:"answer"`     // 参考答案
	Criteria  string    `json:"criteria"`  // 评分要点
	TimeLimit int       `json:"time_limit"` // 建议用时（秒）
	Stage     int       `json:"stage"`      // 阶段 1-16
	Tags      string    `json:"tags"`       // 知识点标签
	CreatedAt time.Time `json:"created_at"`
}

// Task 课堂任务
type Task struct {
	ID          string    `json:"id"`
	Content     string    `json:"content"`
	Deadline    string    `json:"deadline,omitempty"` // 可选截止时间
	CreatedAt   time.Time `json:"created_at"`
}

// TaskFeedback 任务反馈
type TaskFeedback struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	StudentID string    `json:"student_id"`
	FeedbackAt time.Time `json:"feedback_at"`
}

// InteractionState 实时互动状态
type InteractionState struct {
	Mode          string            `json:"mode"`
	AnswererCnt   int               `json:"answerer_cnt"`
	AskerCnt      int               `json:"asker_cnt"`
	Status        string            `json:"status"` // waiting/running/paused/finished
	CurrentIndex  int               `json:"current_index"`  // 当前回答者索引
	CurrentAsker  int               `json:"current_asker"`  // 当前提问者索引（在该回答者组内）
	Answerers     []string          `json:"answerers"`      // 回答者ID列表
	AskersMap     map[int][]string  `json:"askers_map"`     // map[answererIndex][]askerIDs
	Question      *Question         `json:"question"`
	StartedAt     time.Time         `json:"started_at"`
}

// ---- API请求/响应结构 ----

type CreateStudentReq struct {
	Name      string `json:"name" binding:"required"`
	StudentNo string `json:"student_no"`
}

type ImportStudentsReq struct {
	Students []CreateStudentReq `json:"students" binding:"required"`
}

type StartCallReq struct {
	Mode        string `json:"mode" binding:"required"` // multi_one / multi_multi
	AnswererCnt int    `json:"answerer_cnt" binding:"required,min=1"`
	AskerCnt    int    `json:"asker_cnt" binding:"required,min=1"`
}

type GenerateQuestionReq struct {
	Stage int    `json:"stage" binding:"required,min=1,max=16"`
	Tags  string `json:"tags" binding:"required"`
}

type PublishTaskReq struct {
	Content  string `json:"content" binding:"required"`
	Deadline string `json:"deadline,omitempty"`
}

type TaskFeedbackReq struct {
	TaskID string `json:"task_id" binding:"required"`
}

// WebSocket消息类型
type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// 课程阶段定义
var StageMap = map[int]struct {
	Name     string
	Tags     []string
	Level    string
}{
	1:  {"大模型开发入门", []string{"Ollama", "Python调用API", "Streamlit"}, "基础认知"},
	2:  {"Python语言进阶", []string{"面向对象", "网络编程", "闭包/装饰器"}, "基础认知"},
	3:  {"数据处理与统计分析", []string{"Pandas", "MySQL", "数据可视化"}, "基础认知"},
	4:  {"机器学习基础", []string{"KNN", "线性回归", "决策树", "集成学习"}, "基础认知"},
	5:  {"深度学习基础", []string{"神经网络", "反向传播", "Pytorch", "CNN/RNN"}, "原理剖析"},
	6:  {"NLP自然语言处理基础", []string{"Transformer", "BERT", "迁移学习"}, "原理剖析"},
	7:  {"文本分类与模型优化", []string{"FastText", "BERT微调", "量化/剪枝"}, "原理剖析"},
	8:  {"RAG检索增强生成", []string{"LangChain", "向量数据库", "RAG系统"}, "原理剖析"},
	9:  {"Agent智能体开发", []string{"Dify", "CrewAI", "智能体机制"}, "场景应用"},
	10: {"大模型微调", []string{"LoRA", "P-Tuning", "医疗问答"}, "场景应用"},
	11: {"企业级大模型平台", []string{"阿里PAI", "虚拟试衣", "Diffusion"}, "场景应用"},
	12: {"知识图谱与问答系统", []string{"Neo4j", "NER", "关系抽取"}, "场景应用"},
	13: {"NLP高级实战", []string{"BERT+BiLSTM+CRF", "API部署"}, "架构与拓展"},
	14: {"模型部署", []string{"Flask", "Gradio", "Docker容器化", "模型服务封装"}, "架构与拓展"},
	15: {"图像分析与计算机视觉", []string{"ResNet", "Unet", "多模态基础"}, "架构与拓展"},
	16: {"多模态大模型（AIGC）", []string{"Stable Diffusion", "CLIP", "图像生成"}, "架构与拓展"},
}

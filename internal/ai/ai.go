package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"rollcall-pro/internal/models"
)

type MiniMaxClient struct {
	APIKey string
}

func NewMiniMaxClient(apiKey string) *MiniMaxClient {
	return &MiniMaxClient{APIKey: apiKey}
}

type GenerateRequest struct {
	Model      string `json:"model"`
	Messages   []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Temperature float64 `json:"temperature"`
}

type GenerateResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (c *MiniMaxClient) GenerateInterviewQuestion(stage int, tags string) (*models.Question, error) {
	stageInfo, ok := models.StageMap[stage]
	if !ok {
		return nil, fmt.Errorf("invalid stage: %d", stage)
	}

	systemPrompt := `你是一个资深面试官，擅长生成企业级面试题。要求：
1. 生成一道高质量的面试题，难度对应指定阶段
2. 必须包含：题目内容、参考答案、评分要点、建议用时（秒）
3. 格式用JSON返回，如：{"content":"题目","answer":"参考答案","criteria":"评分要点","time_limit":60}
4. 题目要有区分度，能考察真实能力`

	userPrompt := fmt.Sprintf("阶段%d：【%s】\n知识点：%s\n难度：%s\n请生成一道企业面试题。",
		stage, stageInfo.Name, tags, stageInfo.Level)

	req := GenerateRequest{
		Model: "MiniMax-Text-01",
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.7,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// 这里需要根据实际情况调用MiniMax API
	// 由于没有真实APIkey，返回一个模拟题目用于演示
	question := &models.Question{
		Content:   fmt.Sprintf("请简述%s的核心原理，并说明它在实际项目中的应用场景。", tags),
		Answer:    "（参考答案要点：1.核心原理 2.关键步骤 3.实际应用 4.注意事项）",
		Criteria:  "1.理解正确(30%) 2.表达清晰(30%) 3.有实践经验(40%)",
		TimeLimit: 120,
		Stage:     stage,
		Tags:      tags,
	}

	// TODO: 实际调用API
	// resp, err := c.callAPI(body)
	// if err != nil {
	//     return nil, err
	// }
	// return c.parseResponse(resp)

	_ = body // 消除未使用警告
	return question, nil
}

func (c *MiniMaxClient) callAPI(body []byte) (*GenerateResponse, error) {
	req, err := http.NewRequest("POST", "https://api.minimax.chat/v1/text/chatcompletion_pro", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *MiniMaxClient) parseResponse(resp *GenerateResponse) (*models.Question, error) {
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from AI")
	}
	content := resp.Choices[0].Message.Content

	var question models.Question
	if err := json.Unmarshal([]byte(content), &question); err != nil {
		question.Content = content
	}
	return &question, nil
}

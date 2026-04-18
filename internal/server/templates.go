package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

type templateApplyRequest struct {
	Template string `json:"template"`
}

type templateFile struct {
	Path    string
	Content string
}

func projectTemplateFiles(name string) ([]templateFile, error) {
	switch name {
	case "urban-fantasy":
		return []templateFile{
			{Path: filepath.Join("characters", "主角.md"), Content: "# 角色：主角\n- 個性：壓抑、敏銳、習慣先觀察再行動\n- 核心恐懼：失去僅存的重要之人\n- 行為模式：表面冷靜，實際會在關鍵時刻冒險\n- 弱點：不擅長求助\n- 成長限制：除非被迫直面過去，否則不會主動改變\n- 說話風格：短句、保留、情緒不外露\n"},
			{Path: filepath.Join("worldbuilding", "城市規則.md"), Content: "# 都市奇幻世界觀\n\n- 超自然現象只在夜晚顯影\n- 普通人難以長時間記住異象細節\n- 城市不同區域由不同勢力暗中維持秩序\n"},
			{Path: filepath.Join("style", "主線敘事.md"), Content: "# 風格：主線敘事\n- 敘事視角：第三人稱有限視角\n- 句式風格：短句、少修飾、資訊密度高\n- 節奏感：穩定推進\n- 語氣：克制、帶壓力感\n- 禁忌：避免突然變成全知旁白\n"},
			{Path: filepath.Join("chapters", "第01章_開場.md"), Content: "在雨停之前，主角還不打算離開那條巷子。\n"},
		}, nil
	case "mystery":
		return []templateFile{
			{Path: filepath.Join("characters", "偵探.md"), Content: "# 角色：偵探\n- 個性：理性、固執、對細節偏執\n- 核心恐懼：錯失真相導致悲劇重演\n- 行為模式：先蒐證、後推理，不輕信口供\n- 弱點：對他人情感反應遲鈍\n- 成長限制：除非案件牽涉私人創傷，否則不會打破原則\n- 說話風格：精準、節制、帶一點冷意\n"},
			{Path: filepath.Join("worldbuilding", "案件背景.md"), Content: "# 推理案件背景\n\n- 案件發生在封閉社區\n- 主要證詞之間互相矛盾\n- 真相受過去未解案件影響\n"},
			{Path: filepath.Join("style", "懸疑敘事.md"), Content: "# 風格：懸疑敘事\n- 敘事視角：近距離第三人稱\n- 句式風格：段落短、資訊逐步揭露\n- 節奏感：緩慢堆疊，關鍵點突然收緊\n- 語氣：壓抑、冷感、保留線索\n- 禁忌：避免過早解釋所有動機\n"},
			{Path: filepath.Join("chapters", "第01章_現場.md"), Content: "封鎖線外的人群安靜得異常，像每個人都怕自己說錯一句話。\n"},
		}, nil
	default:
		return nil, fmt.Errorf("未知的專案模板：%s", name)
	}
}

func (s *Server) handleApplyTemplate(c *gin.Context) {
	var req templateApplyRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	files, err := projectTemplateFiles(strings.TrimSpace(req.Template))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created := make([]string, 0, len(files))
	for _, file := range files {
		path := filepath.Join(s.cfg.DataDir, file.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.WriteFile(path, []byte(file.Content), 0644); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		created = append(created, file.Path)
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"created": created,
		"message": "專案模板已套用，記得重新索引。",
	})
}

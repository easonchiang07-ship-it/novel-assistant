package server

import (
	"net/http"
	"novel-assistant/internal/workspace"
	"os"

	"github.com/gin-gonic/gin"
)

var ensureWorkspaceIndex = workspace.EnsureIndex
var saveWorkspaceIndex = workspace.SaveIndex

func (s *Server) handleListProjects(c *gin.Context) {
	idx, err := ensureWorkspaceIndex()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, idx)
}

func (s *Server) handleCreateProject(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := workspace.ValidateName(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Name == "default" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "default 是保留名稱"})
		return
	}

	idx, err := ensureWorkspaceIndex()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if workspace.ContainsName(idx, req.Name) {
		c.JSON(http.StatusConflict, gin.H{"error": "專案名稱已存在"})
		return
	}

	dataDir := workspace.ProjectDataDir(req.Name)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	idx.Names = append(idx.Names, req.Name)
	if err := saveWorkspaceIndex(idx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"name": req.Name})
}

func (s *Server) handleSwitchProject(c *gin.Context) {
	name := c.Param("name")

	idx, err := ensureWorkspaceIndex()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !workspace.ContainsName(idx, name) {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到專案：" + name})
		return
	}
	newState, err := s.loadProjectState(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	idx.Active = name
	if err := saveWorkspaceIndex(idx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.setProjectState(newState)
	s.applyProjectSettings()
	c.JSON(http.StatusOK, gin.H{"active": name})
}

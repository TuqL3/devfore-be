package courses

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/devforge/be/internal/courses/adapter/repo"
	"github.com/devforge/be/internal/courses/adapter/rest"
	"github.com/devforge/be/internal/courses/usecase"
)

type Module struct {
	handler *rest.Handler
}

func New(db *gorm.DB) *Module {
	uc := usecase.NewCourses(repo.NewCourseRepo(db))
	return &Module{handler: rest.NewHandler(uc)}
}

func (m *Module) Routes(api *gin.RouterGroup, required, optional gin.HandlerFunc) {
	h := m.handler
	api.GET("/levels", h.Levels)

	g := api.Group("/courses")
	g.GET("", h.List)
	g.GET("/:slug", optional, h.Detail)
	g.GET("/:slug/reviews", h.Reviews)
	g.GET("/:slug/leaderboard", h.Leaderboard)
	g.GET("/:slug/status", optional, h.Status)
	g.GET("/:slug/labs/:labSlug", h.Lab)
	g.POST("/:slug/enroll", required, h.Enroll)
	g.DELETE("/:slug/enroll", required, h.Unenroll)
}

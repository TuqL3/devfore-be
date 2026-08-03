package courses

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/devforge/be/internal/courses/adapter/repo"
	"github.com/devforge/be/internal/courses/adapter/rest"
	"github.com/devforge/be/internal/courses/usecase"
)

type Config struct {
	// Where cover images land, and the origin /uploads is served from. Shared
	// with avatars: one directory, one static route.
	UploadDir string
	PublicURL string
}

type Module struct {
	handler *rest.Handler
}

func New(db *gorm.DB, cfg Config) *Module {
	uc := usecase.NewCourses(repo.NewCourseRepo(db))
	return &Module{handler: rest.NewHandler(uc, cfg.UploadDir, cfg.PublicURL)}
}

func (m *Module) Routes(api *gin.RouterGroup, required, optional, admin gin.HandlerFunc) {
	h := m.handler
	api.GET("/levels", h.Levels)

	// Authentication first, then the role: the role check reads what the auth
	// middleware put on the context, so on its own it would let an anonymous
	// request through as a user with no roles.
	adm := api.Group("/admin", required, admin)
	adm.GET("/lab-images", h.AdminLabImages)
	adm.POST("/uploads/image", h.AdminUploadImage)

	a := adm.Group("/courses")
	a.GET("", h.AdminList)
	a.POST("", h.AdminCreate)
	a.PUT("/:id", h.AdminUpdate)
	a.DELETE("/:id", h.AdminDelete)
	a.GET("/:id/labs", h.AdminLabs)
	a.POST("/:id/labs", h.AdminCreateLab)
	a.GET("/:id/reviews", h.AdminReviews)
	a.POST("/:id/reviews", h.AdminCreateReview)

	// Labs and tasks are addressed by their own id rather than nested under the
	// course: they already know which course they belong to, and a path that
	// repeats it is a path that can disagree with itself.
	adm.PUT("/labs/:labID", h.AdminUpdateLab)
	adm.DELETE("/labs/:labID", h.AdminDeleteLab)
	adm.GET("/labs/:labID/tasks", h.AdminTasks)
	adm.POST("/labs/:labID/tasks", h.AdminCreateTask)
	adm.PUT("/tasks/:taskID", h.AdminUpdateTask)
	adm.DELETE("/tasks/:taskID", h.AdminDeleteTask)
	adm.PUT("/reviews/:reviewID", h.AdminUpdateReview)
	adm.DELETE("/reviews/:reviewID", h.AdminDeleteReview)

	// The caller's own enrolments. Outside the /courses group because it is
	// addressed by who is asking, not by which course.
	api.GET("/me/courses", required, h.MyCourses)

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

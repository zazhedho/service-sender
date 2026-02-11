package router

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"service-sender/infrastructure/database"
	emailHandler "service-sender/internal/handlers/http/email"
	menuHandler "service-sender/internal/handlers/http/menu"
	otpHandler "service-sender/internal/handlers/http/otp"
	permissionHandler "service-sender/internal/handlers/http/permission"
	resetHandler "service-sender/internal/handlers/http/reset"
	roleHandler "service-sender/internal/handlers/http/role"
	sessionHandler "service-sender/internal/handlers/http/session"
	userHandler "service-sender/internal/handlers/http/user"
	interfacereset "service-sender/internal/interfaces/reset"
	authRepo "service-sender/internal/repositories/auth"
	menuRepo "service-sender/internal/repositories/menu"
	otpRepo "service-sender/internal/repositories/otp"
	permissionRepo "service-sender/internal/repositories/permission"
	resetRepo "service-sender/internal/repositories/reset"
	roleRepo "service-sender/internal/repositories/role"
	sessionRepo "service-sender/internal/repositories/session"
	userRepo "service-sender/internal/repositories/user"
	emailSvc "service-sender/internal/services/email"
	menuSvc "service-sender/internal/services/menu"
	otpSvc "service-sender/internal/services/otp"
	permissionSvc "service-sender/internal/services/permission"
	resetSvc "service-sender/internal/services/reset"
	roleSvc "service-sender/internal/services/role"
	sessionSvc "service-sender/internal/services/session"
	userSvc "service-sender/internal/services/user"
	"service-sender/middlewares"
	"service-sender/pkg/config"
	"service-sender/pkg/logger"
	"service-sender/pkg/mailer"
	"service-sender/pkg/security"
	"service-sender/utils"
)

type Routes struct {
	App *gin.Engine
	DB  *gorm.DB
}

func (r *Routes) EmailRoutes() {
	sender, err := mailer.NewBrevoSenderFromEnv()
	if err != nil {
		logger.WriteLog(logger.LogLevelError, "Email sender not configured: "+err.Error())
	}

	svc := emailSvc.NewEmailService(sender)
	h := emailHandler.NewEmailHandler(svc)

	email := r.App.Group("/api/email")
	{
		email.POST("/send", h.SendEmail)
	}
}

func NewRoutes() *Routes {
	app := gin.Default()

	app.Use(middlewares.CORS())
	app.Use(gin.CustomRecovery(middlewares.ErrorHandler))
	app.Use(middlewares.SetContextId())
	app.Use(middlewares.RequestLogger())

	app.GET("/healthcheck", func(ctx *gin.Context) {
		logger.WriteLogWithContext(ctx, logger.LogLevelDebug, "ClientIP: "+ctx.ClientIP())
		ctx.JSON(http.StatusOK, gin.H{
			"message": "OK!!",
		})
	})

	return &Routes{
		App: app,
	}
}

func (r *Routes) UserRoutes() {
	blacklistRepo := authRepo.NewBlacklistRepo(r.DB)
	repo := userRepo.NewUserRepo(r.DB)
	rRepo := roleRepo.NewRoleRepo(r.DB)
	pRepo := permissionRepo.NewPermissionRepo(r.DB)
	uc := userSvc.NewUserService(repo, blacklistRepo, rRepo, pRepo)

	// Setup login limiter if Redis is available
	redisClient := database.GetRedisClient()
	var loginLimiter security.LoginLimiter
	if redisClient != nil {
		loginLimiter = security.NewRedisLoginLimiter(
			redisClient,
			utils.GetEnv("LOGIN_ATTEMPT_LIMIT", 5).(int),
			time.Duration(utils.GetEnv("LOGIN_ATTEMPT_WINDOW_SECONDS", 60).(int))*time.Second,
			time.Duration(utils.GetEnv("LOGIN_BLOCK_DURATION_SECONDS", 300).(int))*time.Second,
		)
	}

	h := userHandler.NewUserHandler(uc, loginLimiter)
	mdw := middlewares.NewMiddleware(blacklistRepo, pRepo)

	// Setup register rate limiter
	registerLimit := utils.GetEnv("REGISTER_RATE_LIMIT", 5).(int)
	registerWindowSeconds := utils.GetEnv("REGISTER_RATE_WINDOW_SECONDS", 60).(int)
	if registerWindowSeconds <= 0 {
		registerWindowSeconds = 60
	}
	registerLimiter := middlewares.IPRateLimitMiddleware(
		redisClient,
		"user_register",
		registerLimit,
		time.Duration(registerWindowSeconds)*time.Second,
	)

	user := r.App.Group("/api/user")
	{
		user.POST("/register", registerLimiter, h.Register)
		user.POST("/login", h.Login)
		user.POST("/forgot-password", h.ForgotPassword)
		user.POST("/reset-password", h.ResetPassword)

		userPriv := user.Group("").Use(mdw.AuthMiddleware())
		{
			userPriv.POST("/logout", h.Logout)
			userPriv.GET("", h.GetUserByAuth)
			userPriv.GET("/:id", mdw.PermissionMiddleware("users", "view"), h.GetUserById)
			userPriv.PUT("", h.Update)
			userPriv.PUT("/:id", mdw.PermissionMiddleware("users", "update"), h.UpdateUserById)
			userPriv.PUT("/change/password", h.ChangePassword)
			userPriv.DELETE("", h.Delete)
			userPriv.DELETE("/:id", mdw.PermissionMiddleware("users", "delete"), h.DeleteUserById)

			// Admin create user endpoint (with role selection)
			userPriv.POST("", mdw.PermissionMiddleware("users", "create"), h.AdminCreateUser)
		}
	}

	r.App.GET("/api/users", mdw.AuthMiddleware(), mdw.PermissionMiddleware("users", "list"), h.GetAllUsers)
}

func (r *Routes) RoleRoutes() {
	repoRole := roleRepo.NewRoleRepo(r.DB)
	repoPermission := permissionRepo.NewPermissionRepo(r.DB)
	repoMenu := menuRepo.NewMenuRepo(r.DB)
	svc := roleSvc.NewRoleService(repoRole, repoPermission, repoMenu)
	h := roleHandler.NewRoleHandler(svc)
	blacklistRepo := authRepo.NewBlacklistRepo(r.DB)
	mdw := middlewares.NewMiddleware(blacklistRepo, repoPermission)

	// List endpoints
	r.App.GET("/api/roles", mdw.AuthMiddleware(), mdw.PermissionMiddleware("roles", "list"), h.GetAll)

	// CRUD endpoints
	role := r.App.Group("/api/role").Use(mdw.AuthMiddleware())
	{
		role.POST("", mdw.PermissionMiddleware("roles", "create"), h.Create)
		role.GET("/:id", mdw.PermissionMiddleware("roles", "view"), h.GetByID)
		role.PUT("/:id", mdw.PermissionMiddleware("roles", "update"), h.Update)
		role.DELETE("/:id", mdw.PermissionMiddleware("roles", "delete"), h.Delete)

		// Permission and menu assignment
		role.POST("/:id/permissions", mdw.PermissionMiddleware("roles", "assign_permissions"), h.AssignPermissions)
		role.POST("/:id/menus", mdw.PermissionMiddleware("roles", "assign_menus"), h.AssignMenus)
	}
}

func (r *Routes) PermissionRoutes() {
	repo := permissionRepo.NewPermissionRepo(r.DB)
	svc := permissionSvc.NewPermissionService(repo)
	h := permissionHandler.NewPermissionHandler(svc)
	blacklistRepo := authRepo.NewBlacklistRepo(r.DB)
	mdw := middlewares.NewMiddleware(blacklistRepo, repo)

	// List endpoints
	r.App.GET("/api/permissions", mdw.AuthMiddleware(), mdw.PermissionMiddleware("permissions", "list"), h.GetAll)

	// Get current user's permissions
	r.App.GET("/api/permissions/me", mdw.AuthMiddleware(), h.GetUserPermissions)

	// CRUD endpoints
	permission := r.App.Group("/api/permission").Use(mdw.AuthMiddleware())
	{
		permission.POST("", mdw.PermissionMiddleware("permissions", "create"), h.Create)
		permission.GET("/:id", mdw.PermissionMiddleware("permissions", "view"), h.GetByID)
		permission.PUT("/:id", mdw.PermissionMiddleware("permissions", "update"), h.Update)
		permission.DELETE("/:id", mdw.PermissionMiddleware("permissions", "delete"), h.Delete)
	}
}

func (r *Routes) MenuRoutes() {
	repo := menuRepo.NewMenuRepo(r.DB)
	svc := menuSvc.NewMenuService(repo)
	h := menuHandler.NewMenuHandler(svc)
	blacklistRepo := authRepo.NewBlacklistRepo(r.DB)
	pRepo := permissionRepo.NewPermissionRepo(r.DB)
	mdw := middlewares.NewMiddleware(blacklistRepo, pRepo)

	// Public endpoints for authenticated users
	r.App.GET("/api/menus/active", mdw.AuthMiddleware(), h.GetActiveMenus)
	r.App.GET("/api/menus/me", mdw.AuthMiddleware(), h.GetUserMenus)

	// List endpoints
	r.App.GET("/api/menus", mdw.AuthMiddleware(), mdw.PermissionMiddleware("menus", "list"), h.GetAll)

	// CRUD endpoints
	menu := r.App.Group("/api/menu").Use(mdw.AuthMiddleware())
	{
		menu.POST("", mdw.PermissionMiddleware("menus", "create"), h.Create)
		menu.GET("/:id", mdw.PermissionMiddleware("menus", "view"), h.GetByID)
		menu.PUT("/:id", mdw.PermissionMiddleware("menus", "update"), h.Update)
		menu.DELETE("/:id", mdw.PermissionMiddleware("menus", "delete"), h.Delete)
	}
}

func (r *Routes) OTPRoutes() {
	redisClient := database.GetRedisClient()
	if redisClient == nil {
		logger.WriteLog(logger.LogLevelWarn, "Redis not available, OTP routes will not be registered")
		return
	}

	sender, err := mailer.NewBrevoSenderFromEnv()
	if err != nil {
		logger.WriteLog(logger.LogLevelError, "OTP sender not configured: "+err.Error())
	}

	repo := otpRepo.NewOTPRepository(redisClient)
	svc := otpSvc.NewOTPService(repo, sender, config.LoadOTPConfig())
	h := otpHandler.NewOTPHandler(svc)

	otp := r.App.Group("/api/auth/otp")
	{
		otp.POST("/send", h.SendRegisterOTP)
		otp.POST("/verify", h.VerifyRegisterOTP)
	}
}

func (r *Routes) PasswordResetRoutes() {
	redisClient := database.GetRedisClient()

	sender, err := mailer.NewBrevoSenderFromEnv()
	if err != nil {
		logger.WriteLog(logger.LogLevelError, "Password reset sender not configured: "+err.Error())
	}

	cfg := config.LoadPasswordResetConfig()

	var svc interfacereset.ServicePasswordResetInterface
	if redisClient == nil {
		logger.WriteLog(logger.LogLevelWarn, "Redis not available, password reset verify routes will not be registered")
	} else {
		repo := resetRepo.NewPasswordResetRepository(redisClient)
		svc = resetSvc.NewPasswordResetService(repo, sender, cfg)
	}

	h := resetHandler.NewResetHandler(svc, sender, cfg)

	reset := r.App.Group("/api/auth/reset-password")
	{
		reset.POST("/email", h.SendResetEmail)
		reset.POST("/request", h.RequestReset)
		reset.POST("/verify", h.VerifyReset)
	}
}

func (r *Routes) SessionRoutes() {
	redisClient := database.GetRedisClient()
	if redisClient == nil {
		logger.WriteLog(logger.LogLevelDebug, "Redis not available, session routes will not be registered")
		return
	}

	repo := sessionRepo.NewSessionRepository(redisClient)
	svc := sessionSvc.NewSessionService(repo)
	h := sessionHandler.NewSessionHandler(svc)
	blacklistRepo := authRepo.NewBlacklistRepo(r.DB)
	pRepo := permissionRepo.NewPermissionRepo(r.DB)
	mdw := middlewares.NewMiddleware(blacklistRepo, pRepo)

	// Session management endpoints (authenticated users only)
	sessionGroup := r.App.Group("/api/user").Use(mdw.AuthMiddleware())
	{
		sessionGroup.GET("/sessions", h.GetActiveSessions)
		sessionGroup.DELETE("/session/:session_id", h.RevokeSession)
		sessionGroup.POST("/sessions/revoke-others", h.RevokeAllOtherSessions)
	}

	logger.WriteLog(logger.LogLevelInfo, "Session management routes registered")
}

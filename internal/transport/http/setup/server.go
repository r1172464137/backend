package setup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"uuid"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	hertzconfig "github.com/cloudwego/hertz/pkg/common/config"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/perfect-panel/server/internal/app/migration/schema"
	"github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/pkg/conf"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/orm"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

//go:embed templates/*.html
var templateFS embed.FS

var initStatus = make(chan bool)
var configPath string
var initConfigMu sync.Mutex

// Start serves the first-installation UI on the local setup listener.
func Start(path string) (chan bool, *server.Hertz) {
	// Set the configuration file path
	configPath = path
	engine := newConfigServer(server.WithHostPorts("127.0.0.1:8080"))

	go func() {
		// Start the server
		if err := engine.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	return initStatus, engine
}

func newConfigServer(opts ...hertzconfig.Option) *server.Hertz {
	engine := server.Default(opts...)
	engine.SetHTMLTemplate(template.Must(template.ParseFS(templateFS, "templates/*.html")))
	engine.GET("/init", handleInit)
	engine.POST("/init/config", handleInitConfig)
	engine.POST("/init/database/test", HandleDatabaseTest)
	engine.POST("/init/mysql/test", HandleMySQLTest)
	engine.POST("/init/redis/test", HandleRedisTest)
	engine.NoRoute(func(_ context.Context, ctx *app.RequestContext) {
		ctx.Redirect(http.StatusFound, []byte("/init"))
	})
	return engine
}

func handleInit(_ context.Context, ctx *app.RequestContext) {
	ctx.HTML(http.StatusOK, "index.html", nil)
}

func handleInitConfig(_ context.Context, ctx *app.RequestContext) {
	initConfigMu.Lock()
	defer initConfigMu.Unlock()
	unlock, err := conf.LockInitialization(configPath)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, utils.H{"code": 500, "msg": "Configuration initialization failed", "data": nil})
		return
	}
	defer unlock()
	var cfg config.File
	if err := conf.Load(configPath, &cfg); err != nil {
		ctx.JSON(http.StatusInternalServerError, utils.H{"code": 500, "msg": "Configuration initialization failed", "data": nil})
		return
	}
	if cfg.Administrator != nil && cfg.Administrator.PasswordHash != "" || cfg.JwtAuth.AccessSecret != "" && cfg.DatabaseConfig().Addr != "" {
		ctx.JSON(http.StatusConflict, utils.H{"code": 409, "msg": "Installation already started; restart to recover", "data": nil})
		return
	}
	var request struct {
		AdminEmail    string `json:"adminEmail"`
		AdminPassword string `json:"adminPassword"`

		DatabaseDriver string `json:"databaseDriver"`
		MysqlHost      string `json:"mysqlHost"`
		MysqlPort      string `json:"mysqlPort"`
		MysqlDatabase  string `json:"mysqlDatabase"`
		MysqlUser      string `json:"mysqlUser"`
		MysqlPassword  string `json:"mysqlPassword"`

		RedisHost     string `json:"redisHost"`
		RedisPort     string `json:"redisPort"`
		RedisPassword string `json:"redisPassword"`
	}
	if err := ctx.BindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, utils.H{
			"code": 400,
			"msg":  "Invalid request",
			"data": nil,
		})
		ctx.Abort()
		return
	}
	cfg.Debug = false
	// jwt secret
	cfg.JwtAuth.AccessSecret = uuid.NewV4().String()
	// database
	dbConfig, err := buildDatabaseConfig(request.DatabaseDriver, request.MysqlHost, request.MysqlPort, request.MysqlDatabase, request.MysqlUser, request.MysqlPassword)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, utils.H{
			"code": 400,
			"msg":  err.Error(),
			"data": nil,
		})
		ctx.Abort()
		return
	}
	cfg.SetDatabaseConfig(dbConfig)
	// redis
	cfg.Redis.Host = fmt.Sprintf("%s:%s", request.RedisHost, request.RedisPort)
	cfg.Redis.Pass = request.RedisPassword

	// create database connection
	dbClient := orm.Mysql{Config: dbConfig}
	db, err := orm.ConnectDatabase(dbClient)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, utils.H{
			"code": 500,
			"msg":  "Database connection failed",
			"data": nil,
		})
		ctx.Abort()
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, utils.H{"code": 500, "msg": "Database connection failed", "data": nil})
		return
	}
	defer sqlDB.Close()
	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, utils.H{"code": 500, "msg": "Database connection failed", "data": nil})
		return
	}
	defer conn.Close()
	release, err := lockSetupDatabase(context.Background(), conn, dbConfig)
	if err != nil {
		ctx.JSON(http.StatusConflict, utils.H{"code": 409, "msg": "Another installation is in progress", "data": nil})
		return
	}
	defer release()
	// The optional connectivity-test endpoint is not an authorization gate:
	// every submission independently proves the target database is empty.
	// Existing installations must use the normal 02179 cutover path.
	if err := schema.RequireNoBusinessObjects(context.Background(), db, false); err != nil {
		ctx.JSON(http.StatusConflict, utils.H{"code": 409, "msg": "Database is not provably empty; existing installations require the normal upgrade path", "data": nil})
		ctx.Abort()
		return
	}
	email := identifier.CanonicalEmail(request.AdminEmail)
	if email == "" || request.AdminPassword == "" {
		ctx.JSON(http.StatusBadRequest, utils.H{"code": 400, "msg": "Administrator email and password are required", "data": nil})
		ctx.Abort()
		return
	}
	// Persist only a one-way recovery hash, never the supplied plaintext.
	// If setup dies after schema.Up, normal startup can finish the admin seed.
	cfg.Administrator = &config.AdministratorConfig{Email: email, PasswordHash: password.EncodePassWord(request.AdminPassword)}
	fileData, err := yaml.Marshal(cfg)
	if err != nil || writeConfigAtomic(configPath, fileData) != nil {
		ctx.JSON(http.StatusInternalServerError, utils.H{"code": 500, "msg": "Configuration initialization failed", "data": nil})
		ctx.Abort()
		return
	}
	// migrate database
	if err = schema.Up(dbClient.Driver(), dbClient.MigrationDsn()); err != nil {
		logger.Errorf("[Init Database] Migrate failed: %v", err.Error())
		ctx.JSON(http.StatusOK, utils.H{
			"code": 500,
			"msg":  "Database migration failed",
			"data": nil,
		})
		ctx.Abort()
		return
	}

	// create admin user
	if err = schema.CreateAdminUser(request.AdminEmail, request.AdminPassword, db); err != nil {
		logger.Errorf("[Init Database] Create admin user failed: %v", err.Error())
		ctx.JSON(http.StatusOK, utils.H{
			"code": 500,
			"msg":  "Admin user creation failed",
			"data": nil,
		})
		ctx.Abort()
		return
	}

	// The account is durable; discard its staged hash from the runtime file.
	cfg.Administrator = nil
	fileData, err = yaml.Marshal(cfg)
	if err != nil || writeConfigAtomic(configPath, fileData) != nil {
		ctx.JSON(http.StatusInternalServerError, utils.H{"code": 500, "msg": "Configuration initialization failed", "data": nil})
		ctx.Abort()
		return
	}

	ctx.JSON(http.StatusOK, utils.H{
		"code":   200,
		"msg":    "Configuration initialized",
		"status": true,
	})
	initStatus <- true
}

// Hold a session-scoped database lock from emptiness proof through final
// config write. This protects separate setup processes targeting the same DB.
func lockSetupDatabase(ctx context.Context, conn *sql.Conn, cfg orm.Config) (func(), error) {
	switch orm.NormalizeDriver(cfg.Driver) {
	case orm.DriverMySQL:
		sum := sha256.Sum256([]byte(cfg.Dbname))
		name := fmt.Sprintf("ppanel-init-%x", sum[:16])
		var acquired sql.NullInt64
		if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", name).Scan(&acquired); err != nil {
			return nil, err
		}
		if !acquired.Valid || acquired.Int64 != 1 {
			return nil, fmt.Errorf("setup lock unavailable")
		}
		return func() { _, _ = conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", name) }, nil
	case orm.DriverPostgres:
		const key int64 = 0x5050414e454c
		var acquired bool
		if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&acquired); err != nil {
			return nil, err
		}
		if !acquired {
			return nil, fmt.Errorf("setup lock unavailable")
		}
		return func() { _, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", key) }, nil
	default:
		return nil, fmt.Errorf("unsupported driver")
	}
}

func writeConfigAtomic(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".ppanel-init-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func HandleMySQLTest(ctx context.Context, requestCtx *app.RequestContext) {
	HandleDatabaseTest(ctx, requestCtx)
}

func HandleDatabaseTest(_ context.Context, ctx *app.RequestContext) {
	var request struct {
		Driver   string `json:"driver"`
		Host     string `json:"host"`
		Port     string `json:"port"`
		Database string `json:"database"`
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if err := ctx.BindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, utils.H{
			"code": 400,
			"msg":  "Invalid request",
			"data": nil,
		})
		ctx.Abort()
		return
	}
	var status = true
	var message string
	var tx *sql.DB
	dbConfig, err := buildDatabaseConfig(request.Driver, request.Host, request.Port, request.Database, request.User, request.Password)
	if err != nil {
		ctx.JSON(http.StatusOK, utils.H{
			"code":   200,
			"msg":    err.Error(),
			"status": false,
		})
		return
	}
	db, err := orm.ConnectDatabase(orm.Mysql{Config: dbConfig})
	if err != nil {
		logger.Errorf("connect database failed, err: %v\n", err.Error())
		status = false
		message = "Database connection failed"
		goto result
	}
	tx, err = db.DB()
	if err != nil || tx == nil {
		status = false
		message = "Database connection failed"
		goto result
	}
	defer tx.Close()
	if err := tx.Ping(); err != nil {
		logger.Errorf("ping database failed, err: %v\n", err.Error())
		status = false
		message = "Database connection failed"
	}

	if err := schema.RequireNoBusinessObjects(context.Background(), db, false); err != nil {
		status = false
		message = "Database is not provably empty; existing installations require the normal upgrade path."
		goto result
	}

result:
	ctx.JSON(http.StatusOK, utils.H{
		"code":   200,
		"msg":    message,
		"status": status,
	})
}

func buildDatabaseConfig(driver, host, port, database, user, password string) (orm.Config, error) {
	normalizedDriver := orm.NormalizeDriver(driver)
	switch normalizedDriver {
	case orm.DriverMySQL, orm.DriverPostgres:
	default:
		return orm.Config{}, fmt.Errorf("unsupported database driver: %s", driver)
	}
	cfg := orm.Config{
		Driver:          normalizedDriver,
		Addr:            fmt.Sprintf("%s:%s", host, port),
		Username:        user,
		Password:        password,
		Dbname:          database,
		MaxIdleConns:    10,
		MaxOpenConns:    10,
		ConnMaxLifetime: orm.DefaultConnMaxLifetimeSeconds,
		ConnMaxIdleTime: orm.DefaultConnMaxIdleTimeSeconds,
		SlowThreshold:   orm.DefaultSlowThresholdMs,
	}
	if normalizedDriver == orm.DriverPostgres {
		cfg.Config = orm.DefaultPostgresConfig
	} else {
		cfg.Config = orm.DefaultMySQLConfig
	}
	return cfg, nil
}

func HandleRedisTest(_ context.Context, ctx *app.RequestContext) {
	var request struct {
		Host     string `json:"host"`
		Port     string `json:"port"`
		Password string `json:"password"`
	}
	if err := ctx.BindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, utils.H{
			"code": 400,
			"msg":  "Invalid request",
			"data": nil,
		})
		ctx.Abort()
		return
	}
	if err := config.RedisPing(fmt.Sprintf("%s:%s", request.Host, request.Port), request.Password, 0); err != nil {
		ctx.JSON(http.StatusOK, utils.H{
			"code":   200,
			"msg":    nil,
			"status": false,
		})
		return
	}
	ctx.JSON(http.StatusOK, utils.H{
		"code":   200,
		"msg":    nil,
		"status": true,
	})
}

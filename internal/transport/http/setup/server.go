// Package setup serves the first-installation wizard. While the server has no
// usable configuration, the command line serves this page on the loopback
// interface, on the configured port, instead of the API. The page tests the
// database and Redis the installer enters; on submit the wizard checks the
// input and that the database is empty, writes the configuration file,
// migrates the database and creates the first administrator, then signals
// the caller to start the real server. The file is written before the
// migration, so a migration that fails leaves an installation the next start
// resumes (the bootstrap migrates and seeds the administrator named in the
// file) instead of a database the wizard refuses as already used.
package setup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"
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
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

//go:embed templates/*.html
var templateFS embed.FS

// MinAdminPasswordLength is the shortest administrator password the wizard
// accepts; the page enforces the same, the handler does not trust it.
const MinAdminPasswordLength = 8

// DefaultPort is the port the wizard listens on when the configuration
// names none.
const DefaultPort = 8080

// wizard is one installation: the configuration file it writes, the signal
// to the caller and the guard against two submissions running at once.
type wizard struct {
	configPath string
	// done receives nil once the installation is complete, or the error
	// that stopped the listener. Buffered, so the sender never blocks.
	done chan error

	mu         sync.Mutex
	installing bool
	installed  bool

	// The database steps, replaceable by tests.
	connect      func(orm.Config) (*gorm.DB, error)
	migrate      func(driver, dsn string) error
	createAdmin  func(email, password string, db *gorm.DB) error
	lockDatabase func(context.Context, *sql.Conn, orm.Config) (func() error, error)
	checkEmpty   func(context.Context, *gorm.DB) error
}

func newWizard(configPath string) *wizard {
	return &wizard{
		configPath:   configPath,
		done:         make(chan error, 1),
		connect:      func(cfg orm.Config) (*gorm.DB, error) { return orm.ConnectDatabase(orm.Mysql{Config: cfg}) },
		migrate:      schema.Up,
		createAdmin:  schema.CreateAdminUser,
		lockDatabase: lockSetupDatabase,
		checkEmpty:   func(ctx context.Context, db *gorm.DB) error { return schema.RequireNoBusinessObjects(ctx, db, false) },
	}
}

// Start serves the first-installation UI on 127.0.0.1 and port (DefaultPort
// when zero). The channel receives nil once the installation is complete
// and the caller can start the real server, or the error that stopped the
// listener; the caller decides what to do about that, so no goroutine here
// ends the process.
func Start(path string, port int) (<-chan error, *server.Hertz) {
	w := newWizard(path)
	addr := Address(port)
	engine := newConfigServer(w, server.WithHostPorts(addr))

	go func() {
		defer func() {
			// Hertz's netpoll transport panics when the listener cannot
			// bind.
			if r := recover(); r != nil {
				w.fail(fmt.Errorf("listen on %s: %v", addr, r))
			}
		}()
		if err := engine.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			w.fail(fmt.Errorf("listen on %s: %w", addr, err))
		}
	}()
	logger.Infof("[Setup] the installation wizard is at http://%s/init; on a remote host reach it through an SSH tunnel (ssh -L %d:127.0.0.1:%d <host>) or install with PPANEL_DB and PPANEL_REDIS", addr, listenPort(port), listenPort(port))
	return w.done, engine
}

// Address is the wizard's listen address for port: the loopback interface,
// so the unauthenticated installer is reachable from the host only.
func Address(port int) string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort(port)))
}

func listenPort(port int) int {
	if port <= 0 {
		return DefaultPort
	}
	return port
}

func newConfigServer(w *wizard, opts ...hertzconfig.Option) *server.Hertz {
	engine := server.Default(opts...)
	engine.SetHTMLTemplate(template.Must(template.ParseFS(templateFS, "templates/*.html")))
	engine.GET("/init", handleInit)
	engine.POST("/init/config", w.handleInitConfig)
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

// initRequest is the installer's form.
type initRequest struct {
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

// validate checks the administrator account the page also validates — the
// handler does not trust the page — and returns the canonical email and the
// complaint to show the installer, "" when the account is acceptable.
func (r initRequest) validate() (email, complaint string) {
	email, err := identifier.ValidateEmail(r.AdminEmail, "", false)
	if err != nil {
		return "", "Administrator email is not a valid email address"
	}
	if utf8.RuneCountInString(r.AdminPassword) < MinAdminPasswordLength {
		return "", fmt.Sprintf("Password must be at least %d characters", MinAdminPasswordLength)
	}
	return email, ""
}

// begin claims the installation for one request; it is false while another
// request installs or once the installation is complete.
func (w *wizard) begin() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.installing || w.installed {
		return false
	}
	w.installing = true
	return true
}

// end releases the installation; a successful one signals the caller.
func (w *wizard) end(success bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.installing = false
	if success {
		w.installed = true
		w.signal(nil)
	}
}

// fail reports the error that stopped the listener.
func (w *wizard) fail(err error) {
	w.signal(err)
}

func (w *wizard) signal(err error) {
	select {
	case w.done <- err:
	default:
	}
}

func reply(ctx *app.RequestContext, status int, msg string) {
	ctx.JSON(status, utils.H{
		"code": status,
		"msg":  msg,
		"data": nil,
	})
	ctx.Abort()
}

func (w *wizard) handleInitConfig(_ context.Context, ctx *app.RequestContext) {
	var request initRequest
	if err := ctx.BindJSON(&request); err != nil {
		reply(ctx, http.StatusBadRequest, "Invalid request")
		return
	}
	adminEmail, complaint := request.validate()
	if complaint != "" {
		reply(ctx, http.StatusBadRequest, complaint)
		return
	}
	if !w.begin() {
		reply(ctx, http.StatusConflict, "An installation is already in progress or complete")
		return
	}
	success := false
	defer func() { w.end(success) }()

	unlock, err := conf.LockInitialization(w.configPath)
	if err != nil {
		reply(ctx, 500, "Configuration initialization failed")
		return
	}
	defer func() {
		if err := unlock(); err != nil {
			logger.Errorf("[Init Config] unlock failed: %v", err)
		}
	}()
	var cfg config.File
	if err := conf.Load(w.configPath, &cfg); err != nil {
		logger.Errorf("[Init Config] read %s: %v", w.configPath, err)
		reply(ctx, http.StatusInternalServerError, "Configuration file could not be read")
		return
	}
	if cfg.Administrator != nil && cfg.Administrator.PasswordHash != "" || cfg.JwtAuth.AccessSecret != "" && cfg.DatabaseConfig().Addr != "" {
		reply(ctx, 409, "Installation already started; restart to recover")
		return
	}
	cfg.Debug = false
	// jwt secret
	cfg.JwtAuth.AccessSecret = uuid.NewV4().String()
	// database
	dbConfig, err := buildDatabaseConfig(request.DatabaseDriver, request.MysqlHost, request.MysqlPort, request.MysqlDatabase, request.MysqlUser, request.MysqlPassword, cfg.AppLocation)
	if err != nil {
		reply(ctx, http.StatusBadRequest, err.Error())
		return
	}
	cfg.SetDatabaseConfig(dbConfig)
	// redis
	cfg.Redis.Host = fmt.Sprintf("%s:%s", request.RedisHost, request.RedisPort)
	cfg.Redis.Pass = request.RedisPassword
	cfg.Administrator = &config.AdministratorConfig{Email: adminEmail, PasswordHash: password.EncodePassWord(request.AdminPassword)}

	// create database connection
	db, err := w.connect(dbConfig)
	if err != nil {
		logger.Errorf("[Init Database] connect failed: %v", err.Error())
		reply(ctx, http.StatusInternalServerError, "Database connection failed")
		return
	}
	sqlDB, err := db.DB()
	if err != nil || sqlDB == nil {
		reply(ctx, 500, "Database connection failed")
		return
	}
	defer closeDatabase(sqlDB)
	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		reply(ctx, 500, "Database connection failed")
		return
	}
	defer closeDatabase(conn)
	release, err := w.lockDatabase(context.Background(), conn, dbConfig)
	if err != nil {
		reply(ctx, 409, "Another installation is in progress")
		return
	}
	defer func() {
		if err := release(); err != nil {
			logger.Errorf("[Init Database] release lock failed: %v", err)
		}
	}()
	if err := w.checkEmpty(context.Background(), db); err != nil {
		reply(ctx, 409, "Database is not provably empty; existing installations require the normal upgrade path")
		return
	}
	fileData, err := yaml.Marshal(cfg)
	if err == nil {
		err = writeConfigAtomic(w.configPath, fileData)
	}
	if err != nil {
		logger.Errorf("[Init Config] write %s: %v", w.configPath, err)
		reply(ctx, http.StatusInternalServerError, "Configuration initialization failed")
		return
	}

	dbClient := orm.Mysql{Config: dbConfig}
	if err = w.migrate(dbClient.Driver(), dbClient.MigrationDsn()); err != nil {
		logger.Errorf("[Init Database] Migrate failed: %v", err.Error())
		reply(ctx, http.StatusInternalServerError, "Database migration failed. The configuration was saved: fix the database (or drop and recreate it) and start the server again to resume the installation; restart uses the password supplied to the installer.")
		return
	}

	if err = w.createAdmin(adminEmail, request.AdminPassword, db); err != nil {
		logger.Errorf("[Init Database] Create admin user failed: %v", err.Error())
		reply(ctx, http.StatusInternalServerError, "Admin user creation failed. The configuration was saved: start the server again to resume the installation; restart uses the password supplied to the installer.")
		return
	}

	cfg.Administrator.PasswordHash = ""
	fileData, err = yaml.Marshal(cfg)
	if err == nil {
		err = writeConfigAtomic(w.configPath, fileData)
	}
	if err != nil {
		reply(ctx, 500, "Configuration cleanup failed; restart to recover")
		return
	}
	success = true
	ctx.JSON(http.StatusOK, utils.H{
		"code":   200,
		"msg":    "Configuration initialized",
		"status": true,
	})
}

// Hold a session-scoped database lock from emptiness proof through final
// config write. This protects separate setup processes targeting the same DB.
func lockSetupDatabase(ctx context.Context, conn *sql.Conn, cfg orm.Config) (func() error, error) {
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
		return func() error {
			var released sql.NullInt64
			if err := conn.QueryRowContext(context.Background(), "SELECT RELEASE_LOCK(?)", name).Scan(&released); err != nil {
				return err
			}
			if !released.Valid || released.Int64 != 1 {
				return errors.New("setup lock release failed")
			}
			return nil
		}, nil
	case orm.DriverPostgres:
		const key int64 = 0x5050414e454c
		var acquired bool
		if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&acquired); err != nil {
			return nil, err
		}
		if !acquired {
			return nil, fmt.Errorf("setup lock unavailable")
		}
		return func() error {
			var released bool
			if err := conn.QueryRowContext(context.Background(), "SELECT pg_advisory_unlock($1)", key).Scan(&released); err != nil {
				return err
			}
			if !released {
				return errors.New("setup lock release failed")
			}
			return nil
		}, nil
	default:
		return nil, fmt.Errorf("unsupported driver")
	}
}

func writeConfigAtomic(path string, data []byte) error {
	return conf.WritePrivateAtomic(path, data)
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
		reply(ctx, http.StatusBadRequest, "Invalid request")
		return
	}
	var status = true
	var message string
	var tx *sql.DB
	// Only the connection is tested; the session zone does not matter.
	dbConfig, err := buildDatabaseConfig(request.Driver, request.Host, request.Port, request.Database, request.User, request.Password, orm.DefaultLocation)
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
	if err != nil {
		logger.Errorf("get database connection failed, err: %v\n", err.Error())
		status = false
		message = "Database connection failed"
		goto result
	}
	defer closeDatabase(tx)
	if err := pingDatabase(tx); err != nil {
		logger.Errorf("ping database failed, err: %v\n", err.Error())
		status = false
		message = "Database connection failed"
	}

	if err := schema.RequireNoBusinessObjects(context.Background(), db, false); err != nil {
		status = false
		message = "Database is not provably empty; existing installations require the normal upgrade path."
	}

result:
	ctx.JSON(http.StatusOK, utils.H{
		"code":   200,
		"msg":    message,
		"status": status,
	})
}

// pingDatabase checks that the connection answers within a bounded time, so
// an address that drops packets cannot hold the install page indefinitely.
func pingDatabase(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}

// closeDatabase releases the connection pool a setup request opened. The
// response is already decided by then, so a failure is only logged.
func closeDatabase(db io.Closer) {
	if err := db.Close(); err != nil {
		logger.Errorf("[Init Database] close database connection failed: %v", err.Error())
	}
}

// buildDatabaseConfig is the configuration of the database the installation
// sets up. Its session stores times in zone, the application's zone.
func buildDatabaseConfig(driver, host, port, database, user, password, zone string) (orm.Config, error) {
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
	// A new database stores times in the application's zone from the
	// start. The parameters are written out, so a later AppLocation change
	// cannot silently reinterpret the stored times.
	if normalizedDriver == orm.DriverPostgres {
		cfg.Config = orm.DefaultPostgresQuery(zone)
	} else {
		cfg.Config = orm.DefaultMySQLQuery(zone)
	}
	return cfg, nil
}

// redisTestTimeout bounds the Redis connection test. Without it an address
// that drops packets keeps the install page waiting through every dial retry
// of the client.
const redisTestTimeout = 5 * time.Second

// HandleRedisTest reports whether the Redis server the installer entered
// answers, before the configuration is written.
func HandleRedisTest(ctx context.Context, requestCtx *app.RequestContext) {
	var request struct {
		Host     string `json:"host"`
		Port     string `json:"port"`
		Password string `json:"password"`
	}
	if err := requestCtx.BindJSON(&request); err != nil {
		reply(requestCtx, http.StatusBadRequest, "Invalid request")
		return
	}
	pingCtx, cancel := context.WithTimeout(ctx, redisTestTimeout)
	defer cancel()
	if err := config.RedisPing(pingCtx, fmt.Sprintf("%s:%s", request.Host, request.Port), request.Password, 0); err != nil {
		requestCtx.JSON(http.StatusOK, utils.H{
			"code":   200,
			"msg":    nil,
			"status": false,
		})
		return
	}
	requestCtx.JSON(http.StatusOK, utils.H{
		"code":   200,
		"msg":    nil,
		"status": true,
	})
}

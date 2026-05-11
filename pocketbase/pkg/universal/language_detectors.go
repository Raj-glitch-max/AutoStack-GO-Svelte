package universal

// LanguageDetector matches a set of indicator files to a language
type LanguageDetector struct {
	Files          []string // any of these files must exist
	Language       string
	PackageManager string
	NoRuntime      bool // true for static sites
}

// FrameworkDetector matches a framework within a language
type FrameworkDetector struct {
	PackageKey    string  // key in package.json dependencies
	ImportPattern string  // string to look for in source files
	FilePattern   string  // file that must exist
	Framework     string
	AppType       AppType
}

// PortPattern is a regex pattern to detect the port an app listens on
type PortPattern struct {
	Pattern  string
	Language string
}

// DependencyPattern maps dependency names to managed service types
type DependencyPattern struct {
	Patterns []string
	Service  string
}

// LanguageDetectors — ordered, first match wins
var LanguageDetectors = []LanguageDetector{
	{Files: []string{"go.mod"}, Language: "go", PackageManager: "go modules"},
	{Files: []string{"package.json"}, Language: "nodejs", PackageManager: "npm"},
	{Files: []string{"requirements.txt"}, Language: "python", PackageManager: "pip"},
	{Files: []string{"pyproject.toml"}, Language: "python", PackageManager: "poetry"},
	{Files: []string{"Pipfile"}, Language: "python", PackageManager: "pipenv"},
	{Files: []string{"Gemfile"}, Language: "ruby", PackageManager: "bundler"},
	{Files: []string{"composer.json"}, Language: "php", PackageManager: "composer"},
	{Files: []string{"pom.xml"}, Language: "java", PackageManager: "maven"},
	{Files: []string{"build.gradle"}, Language: "java", PackageManager: "gradle"},
	{Files: []string{"build.gradle.kts"}, Language: "java", PackageManager: "gradle"},
	{Files: []string{"Cargo.toml"}, Language: "rust", PackageManager: "cargo"},
	{Files: []string{"mix.exs"}, Language: "elixir", PackageManager: "mix"},
	{Files: []string{"index.html"}, Language: "static", NoRuntime: true},
}

// FrameworkDetectors — keyed by language
var FrameworkDetectors = map[string][]FrameworkDetector{
	"nodejs": {
		{PackageKey: "express", Framework: "express", AppType: AppTypeAPI},
		{PackageKey: "fastify", Framework: "fastify", AppType: AppTypeAPI},
		{PackageKey: "next", Framework: "nextjs", AppType: AppTypeFullStack},
		{PackageKey: "nuxt", Framework: "nuxt", AppType: AppTypeFullStack},
		{PackageKey: "svelte", Framework: "sveltekit", AppType: AppTypeFullStack},
		{PackageKey: "gatsby", Framework: "gatsby", AppType: AppTypeStaticSite},
		{PackageKey: "@nestjs/core", Framework: "nestjs", AppType: AppTypeAPI},
		{PackageKey: "socket.io", Framework: "socketio", AppType: AppTypeWebSocket},
		{PackageKey: "ws", Framework: "websocket", AppType: AppTypeWebSocket},
		{PackageKey: "koa", Framework: "koa", AppType: AppTypeAPI},
		{PackageKey: "hapi", Framework: "hapi", AppType: AppTypeAPI},
	},
	"python": {
		{ImportPattern: "fastapi", Framework: "fastapi", AppType: AppTypeAPI},
		{ImportPattern: "flask", Framework: "flask", AppType: AppTypeAPI},
		{ImportPattern: "django", Framework: "django", AppType: AppTypeFullStack},
		{ImportPattern: "tornado", Framework: "tornado", AppType: AppTypeAPI},
		{ImportPattern: "celery", Framework: "celery", AppType: AppTypeWorker},
		{ImportPattern: "tensorflow", Framework: "tensorflow", AppType: AppTypeMLInference},
		{ImportPattern: "torch", Framework: "pytorch", AppType: AppTypeMLInference},
		{ImportPattern: "starlette", Framework: "starlette", AppType: AppTypeAPI},
		{ImportPattern: "aiohttp", Framework: "aiohttp", AppType: AppTypeAPI},
	},
	"go": {
		{ImportPattern: "gin-gonic/gin", Framework: "gin", AppType: AppTypeAPI},
		{ImportPattern: "labstack/echo", Framework: "echo", AppType: AppTypeAPI},
		{ImportPattern: "gofiber/fiber", Framework: "fiber", AppType: AppTypeAPI},
		{ImportPattern: "pocketbase/pocketbase", Framework: "pocketbase", AppType: AppTypeFullStack},
		{ImportPattern: "gorilla/websocket", Framework: "gorilla_ws", AppType: AppTypeWebSocket},
		{ImportPattern: "gorilla/mux", Framework: "gorilla_mux", AppType: AppTypeAPI},
		{ImportPattern: "chi", Framework: "chi", AppType: AppTypeAPI},
	},
	"java": {
		{FilePattern: "src/main/resources/application.properties", Framework: "spring_boot", AppType: AppTypeAPI},
		{FilePattern: "src/main/resources/application.yml", Framework: "spring_boot", AppType: AppTypeAPI},
		{PackageKey: "quarkus", Framework: "quarkus", AppType: AppTypeAPI},
		{PackageKey: "micronaut", Framework: "micronaut", AppType: AppTypeAPI},
	},
	"ruby": {
		{FilePattern: "config/routes.rb", Framework: "rails", AppType: AppTypeFullStack},
		{PackageKey: "sinatra", Framework: "sinatra", AppType: AppTypeAPI},
	},
	"php": {
		{FilePattern: "artisan", Framework: "laravel", AppType: AppTypeFullStack},
		{FilePattern: "public/index.php", Framework: "symfony", AppType: AppTypeFullStack},
	},
	"rust": {
		{ImportPattern: "actix-web", Framework: "actix", AppType: AppTypeAPI},
		{ImportPattern: "axum", Framework: "axum", AppType: AppTypeAPI},
		{ImportPattern: "warp", Framework: "warp", AppType: AppTypeAPI},
	},
	"elixir": {
		{ImportPattern: "phoenix", Framework: "phoenix", AppType: AppTypeFullStack},
	},
}

// DependencyPatterns maps dependency names to managed service types
var DependencyPatterns = []DependencyPattern{
	{Patterns: []string{"postgres", "postgresql", "pg", "psycopg2", "pgx", "gorm", "sequelize", "typeorm", "prisma"}, Service: "postgresql"},
	{Patterns: []string{"mysql", "mariadb", "pymysql", "mysqlclient", "mysql2"}, Service: "mysql"},
	{Patterns: []string{"mongodb", "mongoose", "motor", "pymongo"}, Service: "mongodb"},
	{Patterns: []string{"sqlite", "sqlite3", "better-sqlite3"}, Service: "sqlite"},
	{Patterns: []string{"redis", "ioredis", "aioredis", "go-redis", "jedis", "lettuce"}, Service: "redis"},
	{Patterns: []string{"memcached", "pymemcache", "memjs"}, Service: "memcached"},
	{Patterns: []string{"rabbitmq", "amqplib", "pika", "amqp", "amqp091-go"}, Service: "rabbitmq"},
	{Patterns: []string{"kafka", "confluent-kafka", "sarama", "kafkajs"}, Service: "kafka"},
	{Patterns: []string{"elasticsearch", "opensearch", "elastic"}, Service: "opensearch"},
	{Patterns: []string{"s3", "boto3", "aws-sdk", "minio", "@aws-sdk/client-s3"}, Service: "s3"},
}

// PortPatterns are regex patterns to detect the port an app listens on
var PortPatterns = []PortPattern{
	{Pattern: `\.listen\((\d{4,5})`, Language: "nodejs"},
	{Pattern: `ListenAndServe\(".*:(\d{4,5})"`, Language: "go"},
	{Pattern: `run\(.*port=(\d{4,5})`, Language: "python"},
	{Pattern: `server\.port=(\d{4,5})`, Language: "java"},
	{Pattern: `EXPOSE (\d{4,5})`, Language: "dockerfile"},
	{Pattern: `PORT.*=.*(\d{4,5})`, Language: "env"},
	{Pattern: `port.*=.*(\d{4,5})`, Language: "config"},
}

// BaseImageLanguageMap maps Docker base image names to languages
var BaseImageLanguageMap = map[string]string{
	"node":       "nodejs",
	"python":     "python",
	"golang":     "go",
	"openjdk":    "java",
	"eclipse-temurin": "java",
	"php":        "php",
	"ruby":       "ruby",
	"rust":       "rust",
	"elixir":     "elixir",
	"dotnet":     "dotnet",
	"mcr.microsoft.com/dotnet": "dotnet",
	"nginx":      "static",
	"httpd":      "static",
}

// WellKnownServiceNames maps docker-compose service names to managed AWS services
var WellKnownServiceNames = map[string]string{
	"postgres":   "postgresql",
	"postgresql": "postgresql",
	"db":         "postgresql",
	"database":   "postgresql",
	"mysql":      "mysql",
	"mariadb":    "mysql",
	"redis":      "redis",
	"cache":      "redis",
	"mongodb":    "mongodb",
	"mongo":      "mongodb",
	"rabbitmq":   "rabbitmq",
	"kafka":      "kafka",
	"zookeeper":  "", // skip — managed by MSK
	"minio":      "s3",
	"elasticsearch": "opensearch",
	"opensearch": "opensearch",
}

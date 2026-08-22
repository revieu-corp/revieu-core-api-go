# RevieU Backend

Backend services for the RevieU platform.

## ⚠️ 首次克隆后必须执行

```bash
./scripts/setup.sh
```

这会自动安装 Git hooks 防止提交未加密的 secrets 和强制 commit message 规范。

---

## 📂 Project Structure

```
revieu-core-api-go/
├── apps/
│   ├── core/              # Core API Service (Go)
│   └── example-service/   # Placeholder for future services
├── scripts/
│   ├── setup.sh          # 🔥 首次运行这个！
│   ├── seal-secrets.sh   # 加密 secrets
│   └── hooks/            # Git hooks
└── lefthook.yml          # Hook 配置
```

## 🛠 Technology Stack

- **Core Service**: Go 1.24+, Gin, GORM, PostgreSQL
- **Infrastructure**: Kubernetes (k3s), ArgoCD, Sealed Secrets
- **CI/CD**: GitHub Actions

---

## 🚀 Quick Start

### 1. Clone and Setup

```bash
git clone git@github.com:revieu-corp/revieu-core-api-go.git
cd revieu-core-api-go

# 🔥 重要：安装 Git hooks
./scripts/setup.sh
```

### 2. Configure Secrets

```bash
# Copy example secrets
export CLOUDFLARE_ACCOUNT_ID=...
export CLOUDFLARE_KV_NAMESPACE_ID=...
export CLOUDFLARE_API_TOKEN="<CLOUDFLARE_API_TOKEN>"
export JWT_SECRET="<JWT_SECRET>"

# Encrypt before committing
./scripts/seal-secrets.sh
```

### 3. Run Core Service

```bash
cd apps/core

# (first time / schema changes) apply DB migrations
go install github.com/pressly/goose/v3/cmd/goose@latest
make migrate-up GOOSE="" DB_DSN=""

# start API service
go run cmd/app/main.go
```

---

## 🗄️ Manual DB Migration Runbook (Current)

当前部署是容器 + ArgoCD + k3s，但 migration 先手动执行。应用内 `AutoMigrate` 保持关闭，发布前手动跑 Goose。

### 0. One-time setup

```bash
cd apps/core
go install github.com/pressly/goose/v3/cmd/goose@latest
```

### 1. Set connection variables

```bash
export GOOSE="$HOME/go/bin/goose"
export DB_PASSWORD="<DB_PASSWORD>"

export DEV_DSN="<DEV_DATABASE_URL>"
export PRD_DSN="<PROD_DATABASE_URL>"
```

### 2. Add a new schema change (when fields/tables change)

```bash
cd apps/core

# create a new migration file template (does not change DB)
make migrate-create name=add_coupon_scope_fields

# edit the generated file in apps/core/migrations:
# - write SQL under -- +goose Up
# - write rollback SQL under -- +goose Down

# apply migration to dev DB (this step changes DB schema)
make migrate-up GOOSE="$GOOSE" DB_DSN="$DEV_DSN"
```

Notes:
- `make migrate-create` only creates a SQL file template.
- Database schema changes happen when `make migrate-up` runs.

### 3. Standard release flow

```bash
cd apps/core

# check status
make migrate-status GOOSE="$GOOSE" DB_DSN="$DEV_DSN"
make migrate-status GOOSE="$GOOSE" DB_DSN="$PRD_DSN"

# migrate dev first
make migrate-up GOOSE="$GOOSE" DB_DSN="$DEV_DSN"

# verify app behavior, then migrate prod
make migrate-up GOOSE="$GOOSE" DB_DSN="$PRD_DSN"
```

### 4. Baseline rule for existing environments

如果环境已经有历史表，不能直接执行 `00001_init_schema.sql` 的 `up`。先做 baseline（只写 Goose 元数据，不改业务表）：

```bash
cd apps/core
make migrate-status GOOSE="$GOOSE" DB_DSN="$PRD_DSN" # initialize goose_db_version if missing

PGPASSWORD="<DB_PASSWORD>" psql -h "<DB_HOST>" -p 5432 -U "<DB_USER>" -d "<DB_NAME>" -v ON_ERROR_STOP=1 -c \
"INSERT INTO goose_db_version (version_id, is_applied)
 SELECT 1, true
 WHERE NOT EXISTS (
   SELECT 1 FROM goose_db_version WHERE version_id = 1 AND is_applied = true
 );"

make migrate-status GOOSE="$GOOSE" DB_DSN="$PRD_DSN"
```

### 5. Safety notes

- 生产只执行 `migrate-up`，不要自动 `migrate-down`。
- 顺序固定：`dev` -> `prd`。
- 先 migration，后 ArgoCD 发布镜像。

---

## 🔐 Secrets Management

**Never commit unencrypted secrets!** The Git hooks will block you.

### Workflow

1. Edit `apps/core/configs/secrets.yaml`
2. Encrypt: `./scripts/seal-secrets.sh`
3. Commit: `git add apps/core/configs/sealed-secrets.yaml`

See [scripts/README.md](scripts/README.md) for details.

---

## 📝 Commit Message Convention

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

<body>

Closes #<issue-id>

Co-Authored-By: Name <email@example.com>
```

**Types**: feat, fix, docs, style, refactor, perf, test, chore, ci, build, revert

Examples:

- `feat(core): add user authentication`
- `fix(api): resolve null pointer exception`
- `docs(readme): update installation instructions`

The commit-msg hook enforces subject format, non-empty body, issue close trailer, and `Co-Authored-By` trailer.

---

## 🧪 Development

### Run Tests

```bash
cd apps/core
go test ./...
```

### Run with Docker

```bash
docker build -t revieu-core-api-go -f apps/core/build/package/Dockerfile apps/core
docker run -p 8080:8080 revieu-core-api-go
```

---

## 📚 Documentation

- [Database ERD (SVG)](docs/database-erd.svg) - Full database schema with PK/FK markers and FK links
- [Scripts README](scripts/README.md) - Secrets management and hooks
- [Core Service](apps/core/) - API documentation
- [Infrastructure Repo](https://github.com/revieu-corp/revieu-infra) - K8s configs

### Regenerate Database ERD

```bash
python scripts/generate_db_erd_svg.py \
  --host "<DB_HOST>" \
  --user "<DB_USER>" \
  --db "<DB_NAME>" \
  --password "<DB_PASSWORD>" \
  --output docs/database-erd.svg
```

---

## 🤝 Contributing

1. Run `./scripts/setup.sh` first
2. Create a feature branch
3. Follow commit message conventions
4. Encrypt secrets before committing
5. Create a PR to `dev` branch

---

## 📄 License

[Add your license here] test

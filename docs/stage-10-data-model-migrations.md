# 阶段 10：数据模型与数据库约束（含 golang-migrate）

> 改造范围：`schema.sql` → `migrations/`、`api/dbops/api.go`（时间类型、资源泄漏、死代码）。
> 前置：阶段 9（调度器并发）已提交。

## 1. 学习目标

理解「数据库 schema 也是代码，需要版本管理」，以及外键、索引、正确的时间类型如何影响数据正确性和查询性能。

## 2. 现状与问题

| # | 问题 | 位置 | 后果 |
| --- | --- | --- | --- |
| 1 | `schema.sql` 是一次性脚本，无版本管理 | [schema.sql](schema.sql) | 无法安全地演进 schema，无法回滚 |
| 2 | `display_ctime` 用 `VARCHAR(32)` 存时间 | [schema.sql:14](schema.sql:14) | 时间无法排序、无法用时间函数 |
| 3 | `sessions.TTL` 类型（阶段 5 已改 `BIGINT`）尚未固化进迁移 | [schema.sql:28](schema.sql:28) | 改动散落在口头约定里 |
| 4 | 无外键、无索引 | [schema.sql](schema.sql) | 孤儿评论/孤儿视频，列表查询随数据变慢 |
| 5 | `ListAllVideos` 忘 `defer rows.Close()` | [api/dbops/api.go:322](api/dbops/api.go) | 连接泄漏 |
| 6 | `ListComments`（带时间参数的）是死代码且把评论 `id` 填进 `VideoId` | [api/dbops/api.go:149](api/dbops/api.go) | 无用且误导 |

## 3. 分步骤改法

### 步骤 1：引入 golang-migrate

```powershell
go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

（或用 `go run` 方式，避免全局安装依赖版本。）

### 步骤 2：把 schema 拆成迁移文件

新建 `migrations/000001_init.up.sql`，内容是在当前 `schema.sql` 基础上，把阶段 5/6 的改动一起固化（`TTL BIGINT`、时间列类型、外键、索引）：

```sql
CREATE TABLE IF NOT EXISTS users (
  id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  login_name VARCHAR(64) NOT NULL UNIQUE,
  pwd VARCHAR(64) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS video_info (
  id VARCHAR(64) PRIMARY KEY,
  author_id INT UNSIGNED NOT NULL,
  title VARCHAR(64) NOT NULL,
  display_ctime DATETIME NOT NULL,
  CONSTRAINT fk_video_author FOREIGN KEY (author_id) REFERENCES users(id),
  INDEX idx_video_author (author_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS comments (
  id VARCHAR(64) PRIMARY KEY,
  video_id VARCHAR(64) NOT NULL,
  author_id INT UNSIGNED NOT NULL,
  content TEXT NOT NULL,
  create_time DATETIME NOT NULL,
  CONSTRAINT fk_comment_video FOREIGN KEY (video_id) REFERENCES video_info(id) ON DELETE CASCADE,
  CONSTRAINT fk_comment_author FOREIGN KEY (author_id) REFERENCES users(id),
  INDEX idx_comment_video (video_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS sessions (
  session_id VARCHAR(64) PRIMARY KEY,
  TTL BIGINT NOT NULL,
  login_name VARCHAR(64) NOT NULL,
  INDEX idx_session_login (login_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS video_del_rec (
  video_id VARCHAR(64) PRIMARY KEY
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

配套 `migrations/000001_init.down.sql`（反操作：`DROP TABLE`，注意顺序，先删子表再删父表）。

删掉根目录 `schema.sql`（或保留一份注释指向 `migrations/`）。

### 步骤 3：时间列改 `DATETIME`，代码同步

`display_ctime` 从 `VARCHAR(32)` 改成 `DATETIME`。`api/dbops/api.go` 的 `AddVideo` 里：

```go
ctime := t.Format("Jan 2 2006, 15:04:05")
```

这个格式是「给人看的」，不适合存库。改成存标准时间：

```go
ctime := time.Now()
// INSERT 时传 ctime（time.Time），列是 DATETIME，driver 会正确转换
```

`defs.VideoInfo.DisplayCtime` 的 JSON 字段仍然是字符串，格式化的责任交给「读出来之后再格式化」，而不是存一个已格式化的字符串进库。

> 如果暂时不想动「展示格式」，最低限度是把列改成 `DATETIME`、存 `time.Now()`，读出来时 `Scan` 进 `time.Time` 再 `Format`。关键是**库里的时间是真正的 DATETIME，不是字符串**。

### 步骤 4：修数据访问层的收尾问题

- `ListAllVideos` 补 `defer rows.Close()`。
- `ListComments`（带 `originTime, endTime` 参数的）是死代码——handler 用的是 `ListCommentsByVideo`，直接删除整个函数，顺带消掉「评论 id 填进 VideoId」这个误导性 bug。

用 `rg "ListComments\(" api` 确认没有调用方后再删。

### 步骤 5：跑迁移

```powershell
migrate -path ./migrations -database "mysql://root:xxx@tcp(localhost:3306)/streamhub" up
```

（密码从 `.env` 读，不要写死在命令里。）

## 4. 验收标准

- [ ] `migrations/000001_init.up.sql` 和 `.down.sql` 存在，`migrate up` 能建出完整库。
- [ ] 根目录 `schema.sql` 已移除或指向迁移目录。
- [ ] `display_ctime` 是 `DATETIME`，代码读写一致，无字符串存时间。
- [ ] 外键与索引在迁移文件中定义（`video_info.author_id`、`comments.video_id`、`comments.author_id`、`sessions.login_name`）。
- [ ] `ListAllVideos` 关闭 rows；死代码 `ListComments` 已删。
- [ ] 三件套通过。

## 5. 提交建议

```powershell
git add migrations api/dbops/api.go
git rm schema.sql
git commit -m "feat: manage schema with golang-migrate and add constraints"
```

## 6. 拓展思考题

1. `migrate up` 和 `migrate down` 各做什么？`down` 文件的顺序为什么重要？
2. 外键 `ON DELETE CASCADE` 意味着什么？删一个视频，它的评论会怎样？这个行为是否符合业务预期？
3. 为什么「展示格式」应该在读出来之后再格式化，而不是存库前格式化？
4. 加了索引之后，哪些查询变快了？索引为什么会让写入变慢？这就是「读写权衡」。
5. `ListComments` 这种「字段错位的死代码」为什么比「没写某个功能」更危险？它传递了什么错误信号？

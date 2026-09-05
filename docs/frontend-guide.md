# StreamHub 前端微服务改造指南（方案 A）

> 目标架构：
>
> - `api`（8080）：注册/登录 JSON 接口 + 托管静态前端页面
> - `streamsever`（9000）：视频上传、播放（需要加 CORS）
> - `scheduler`（9001）：视频删除记录 + 定时清理（先修到能编译、能手动验证）
> - `web` 服务：整个删除，Cookie/HomeHandler 那套逻辑作废
>
> 会话方式：**登录态不用 Cookie，改用「登录接口返回的 session_id 存 localStorage，
> 之后通过请求头 X-Session-Id 传给后端」**。这是方案 A 的核心。

## 第 0 步：先修 scheduler 的编译错误（和前端无关，但不修整个项目起不来）

### 0.1 `scheduler/taskrunner/defs.go`

把文件最后一行的半截代码删掉：

```go
type message
```

删完后这个文件应该是类型定义 + 常量，没有 `message` 这个东西。

### 0.2 `scheduler/dbops/api.go`

两处修改：

1. import 里删掉没有用到的 mysql 驱动（connection.go 里已经用 `_` 导过了）：

```go
import "log"
```

2. SQL 少了 `VALUES (?)`，补上：

```go
stmtIn, err := dbConnection.Prepare("INSERT INTO video_del_rec (video_id) VALUES (?)")
```

### 0.3 `scheduler/dbops/internal.go`

删掉 `ReadVideoDeletionRecord` 里的 `defer dbConnection.Close()`。
它会关掉全局唯一的连接，第一次读之后整个 scheduler 的数据库操作全部失败。

### 0.4 `scheduler/taskrunner/task.go`

**问题**：`VedioClearDispatcher(dc DataChannel, count int) error` 有两个参数，
但 `Function` 类型（defs.go）只允许 `func(dc DataChannel) error`。改成：

```go
func VideoClearDispatcher(dc DataChannel) error {
	ids, err := dbops.ReadVideoDeletionRecord(3)
	if err != nil {
		log.Printf("VideoClearDispatcher error: %v", err)
		return err
	}
	if len(ids) == 0 {
		return errors.New("VideoClearDispatcher is empty.")
	}
	for _, id := range ids {
		dc <- id
	}
	return nil
}
```

顺手把 `VideoClearExecuter` 里的并发删掉（现在是 goroutine 还没跑完就开始查 errMap，
错误会漏报）。改成逐个删除：

```go
func VideoClearExecuter(dc DataChannel) error {
	errMap := &sync.Map{}
loop:
	for {
		select {
		case id := <-dc:
			if err := DeleteVideo(id.(string)); err != nil {
				errMap.Store(id, err)
				continue
			}
			if err := dbops.DeleteVideoDeletionRecord(id.(string)); err != nil {
				errMap.Store(id, err)
			}
		default:
			break loop
		}
	}
	var result error
	errMap.Range(func(k, v interface{}) bool {
		if e, ok := v.(error); ok && e != nil {
			result = e
			return false
		}
		return true
	})
	return result
}
```

另外建议把包级的 `var err error` 删掉（上面已经改用局部变量 `result`），
避免多 goroutine 共享变量的数据竞争。

### 0.5 `scheduler/taskrunner/tsmain.go`

`Start()` 现在是废代码。改成真正装配你的两个函数：

```go
func Start() {
	r := CreateNewRunner(3, false, VideoClearDispatcher, VideoClearExecuter)
	go r.StartDispatch()
}
```

> 说明：dispatcher 在没记录时返回 error → Runner 发 CLOSE → 一轮结束。
> 要做到"每隔 N 秒自动清理一轮"，后续接 `Worker`（tsmain.go 里已有雏形），
> 那是下一个阶段的事，先保证能跑一轮。

### 验证第 0 步

在仓库根目录执行：

```powershell
go build ./scheduler
```

没有输出就是通过。api 和 streamsever 本来就能编译，可顺带全查：

```powershell
go build ./api ./scheduler ./streamsever
```

## 第 1 步：删掉 web 服务，解决 8080 冲突 + 空文件编译错误

web 服务被 api 服务取代（api 也是 8080）。在仓库根目录执行：

```powershell
git rm -r web
```

这会同时把 [web/client.go](C:/Users/Rafael/StreamHub/web/client.go)（0 字节空文件）
等全部移除，编译错误 `expected 'package', found 'EOF'` 也就消失了。
`build.sh` 是 Linux 课程的残留脚本，本机不用，可以一并删除或不管。

## 第 2 步：整理前端文件目录

统一约定：**所有服务都从仓库根目录启动**（`go run ./api` 这种），
代码里的相对路径（`./static`、`./videos`）都以仓库根目录为基准。

在仓库根目录执行：

```powershell
Rename-Item templetes static
New-Item -ItemType Directory videos
```

最终目录结构（新增的你自己创建）：

```text
StreamHub/
├── api/
├── scheduler/
├── streamsever/
├── static/                 <- 原 templetes 改名，放前端页面
│   ├── index.html          <- 你新建：登录/注册页
│   └── userhome.html       <- 你新建：登录后的上传页
├── videos/                 <- 你新建：上传的视频落盘到这里
├── schema.sql              <- 你新建：建表脚本（第 5 步）
└── docs/frontend-guide.md
```

## 第 3 步：api 服务托管静态页面（改 `api/main.go`）

在 `RegisterHandlers` 里、注册接口之前，加三行静态托管：

```go
router.Static("/static", "./static")                      // /static/xxx → ./static/xxx
router.StaticFile("/", "./static/index.html")             // 访问 http://localhost:8080/
router.StaticFile("/userhome", "./static/userhome.html")  // 登录后跳转的页面
```

`StaticFile` 会自动处理 GET/HEAD，无需再写 HomeHandler。
改完的 `RegisterHandlers` 大致是：

```go
func RegisterHandlers() *gin.Engine {
	router := gin.Default()

	router.Static("/static", "./static")
	router.StaticFile("/", "./static/index.html")
	router.StaticFile("/userhome", "./static/userhome.html")

	router.POST("/user", CreateUser)      // 注册
	router.POST("/user/login", Login)     // 登录

	api := router.Group("/api")           // 将来需要登录态的接口放这里
	api.Use(SessionMiddleware)            // 校验 X-Session-Id

	return router
}
```

> 为什么页面由 api 托管而不是单独再起一个服务？
> 因为这样浏览器里的 fetch 请求（`/user`、`/user/login`）和页面**同源**，
> 不需要 CORS，最简单。跨端口只剩「上传/播放」要到 9000，见第 4 步。

## 第 4 步：streamsever 加 CORS（新建 `streamsever/cors.go`）

上传请求从 8080 的页面发到 9000，属于跨域。新建文件：

```go
package main

import "github.com/gin-gonic/gin"

func CorsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, X-Session-Id")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204) // 预检请求直接放行
			return
		}
		c.Next()
	}
}
```

然后在 [streamsever/main.go](C:/Users/Rafael/StreamHub/streamsever/main.go) 的
`RegisterHandlers` 里挂上：

```go
router.Use(LimiterMiddleware(10))
router.Use(CorsMiddleware())
```

> CORS 要点：
> - 带自定义请求头（比如 `X-Session-Id`）的请求会先发一个 `OPTIONS` 预检，
>   中间件要在预检时直接返回 204，否则浏览器会拦。
> - 播放 `<video src="http://localhost:9000/videos/xxx">` 不受 CORS 限制，
>   但用 fetch 读取 9000 的响应必须要有 `Access-Control-Allow-Origin`。

## 第 5 步：建表（新建根目录 `schema.sql`）

```sql
CREATE DATABASE IF NOT EXISTS streamhub DEFAULT CHARACTER SET utf8mb4;
USE streamhub;

CREATE TABLE IF NOT EXISTS users (
  id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  login_name VARCHAR(64) NOT NULL UNIQUE,
  pwd VARCHAR(64) NOT NULL
);

CREATE TABLE IF NOT EXISTS video_info (
  id VARCHAR(64) PRIMARY KEY,
  author_id INT NOT NULL,
  title VARCHAR(64) NOT NULL,
  display_ctime VARCHAR(32)
);

CREATE TABLE IF NOT EXISTS comments (
  id VARCHAR(64) PRIMARY KEY,
  video_id VARCHAR(64) NOT NULL,
  author_id INT NOT NULL,
  content TEXT NOT NULL,
  create_time DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  session_id VARCHAR(64) PRIMARY KEY,
  TTL VARCHAR(8) NOT NULL,
  login_name VARCHAR(64) NOT NULL
);

CREATE TABLE IF NOT EXISTS video_del_rec (
  video_id VARCHAR(64) PRIMARY KEY
);
```

执行（密码是 `api/.env` 里 `MYSQL_PWD` 的值）：

```powershell
& "C:\Program Files\MySQL\MySQL Server 8.0\bin\mysql.exe" -u root -p < schema.sql
```

> 注意 `sessions` 表的列叫 `login_name`；api/dbops/internal.go 的
> `RetrieveSession` 里写的是 `user_name`，目前没被调用，先记住这个坑，别去用。

## 第 6 步：写登录/注册页（新建 `static/index.html`）

核心逻辑：点按钮 → `fetch` 发 JSON → 成功就把 `session_id` 存 localStorage →
跳转 `/userhome`；失败就把后端的 `{"error":"..."}` 显示出来。

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <title>StreamHub</title>
</head>
<body>
  <h1>StreamHub</h1>
  <h2 id="form-title">登录</h2>
  <input id="user_name" placeholder="用户名"><br>
  <input id="pwd" type="password" placeholder="密码"><br>
  <button id="submit-btn" onclick="submit()">登录</button>
  <p><a href="#" onclick="toggle()">没有账号？去注册</a></p>
  <p id="msg"></p>

  <script>
    let isLogin = true;

    function toggle() {
      isLogin = !isLogin;
      document.getElementById("form-title").textContent = isLogin ? "登录" : "注册";
      document.getElementById("submit-btn").textContent = isLogin ? "登录" : "注册";
    }

    async function submit() {
      const user_name = document.getElementById("user_name").value;
      const pwd = document.getElementById("pwd").value;
      const msg = document.getElementById("msg");

      const res = await fetch(isLogin ? "/user/login" : "/user", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ user_name, pwd }),  // 字段名必须和 Go 的 json tag 一致
      });

      const data = await res.json().catch(() => ({}));

      if (res.ok) {
        localStorage.setItem("session_id", data.session_id);
        localStorage.setItem("user_name", user_name);
        location.href = "/userhome";
      } else {
        msg.textContent = data.error || "请求失败";
      }
    }
  </script>
</body>
</html>
```

对照 Go 代码记牢三处约定：

1. 请求字段：`user_name`、`pwd`（见 [apidefs.go](C:/Users/Rafael/StreamHub/api/defs/apidefs.go:4)）。
2. 成功响应：`{"success":true,"session_id":"..."}`。
3. 失败响应：`{"error":"...","error_code":"..."}`（见 [error.go](C:/Users/Rafael/StreamHub/api/defs/error.go)）。

## 第 7 步：写上传页（新建 `static/userhome.html`）

核心逻辑：加载时检查 localStorage 有没有 session（没有就踢回首页）→
选文件 → `FormData` 带上 `file` 字段 POST 到 9000 → 成功后把 9000 的播放地址塞进
`<video>`。

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <title>StreamHub 用户主页</title>
</head>
<body>
  <h1>你好，<span id="who"></span></h1>
  <button onclick="logout()">退出登录</button>
  <hr>
  <input type="file" id="file-input">
  <button onclick="upload()">上传视频</button>
  <p id="msg"></p>
  <video id="player" controls width="640" hidden></video>

  <script>
    const sessionId = localStorage.getItem("session_id");
    if (!sessionId) {
      location.href = "/";   // 没有登录态就回登录页
    }
    document.getElementById("who").textContent = localStorage.getItem("user_name");

    async function upload() {
      const fileInput = document.getElementById("file-input");
      if (!fileInput.files[0]) { alert("请先选择文件"); return; }

      const vid = crypto.randomUUID();   // 生成视频 id
      const formData = new FormData();
      formData.append("file", fileInput.files[0]);  // 字段名必须叫 file

      const res = await fetch("http://localhost:9000/upload/" + vid, {
        method: "POST",
        body: formData,     // 不要手动设 Content-Type，让浏览器带 boundary
      });

      const msg = document.getElementById("msg");
      if (res.ok) {
        msg.textContent = "上传成功，视频 id：" + vid;
        const player = document.getElementById("player");
        player.src = "http://localhost:9000/videos/" + vid;
        player.hidden = false;
      } else {
        msg.textContent = "上传失败：" + res.status;
      }
    }

    function logout() {
      localStorage.removeItem("session_id");
      localStorage.removeItem("user_name");
      location.href = "/";
    }
  </script>
</body>
</html>
```

对应后端逻辑：`/upload/:vid-id` 的 UploadHandler 用 `context.FormFile("file")`
拿文件，存到 `VIDEO_DIR + vid`（根目录 `./videos/`），播放用
`http.ServeContent` 支持拖动进度条。

## 第 8 步：让 fetch 带上登录态（给将来受保护接口的模板）

目前 `api` 的 `/api` 组挂了 `SessionMiddleware`，但还没有具体路由。将来加一个
需要登录的接口时，后端直接写进 `api` 组即可；前端每次请求带上：

```js
fetch("/api/xxx", {
  headers: {
    "X-Session-Id": localStorage.getItem("session_id"),
  },
});
```

后端用 `context.GetHeader("X-Session-Id")` 读取，就是
[auth.go](C:/Users/Rafael/StreamHub/api/auth.go:8) 里已经写好的校验逻辑。
这也是方案 A 和旧 Cookie 方案的本质区别：**状态存在浏览器 localStorage，
身份靠请求头传递**。

## 第 9 步：完整验证清单

全部从仓库根目录操作：

```powershell
# 1. 编译全绿
go build ./api ./scheduler ./streamsever

# 2. 三个终端分别启动
go run ./api          # http://localhost:8080
go run ./streamsever  # http://localhost:9000
go run ./scheduler    # http://localhost:9001
```

接口层先 curl 验证（PowerShell 里用 curl.exe，别用别名）：

```powershell
curl.exe -X POST http://localhost:8080/user -H "Content-Type: application/json" -d "{\"user_name\":\"t1\",\"pwd\":\"123\"}"
curl.exe -X POST http://localhost:8080/user/login -H "Content-Type: application/json" -d "{\"user_name\":\"t1\",\"pwd\":\"123\"}"
```

浏览器层按顺序走：

1. 打开 `http://localhost:8080/`，看到登录/注册页。
2. 切到注册，注册一个新用户，页面应自动跳到 `/userhome`。
3. 上传一个小视频，出现播放器并能拖动进度条。
4. 上传完，根目录 `videos/` 下多了一个以 UUID 命名的文件。
5. 点退出登录，回到首页；此时直接访问 `/userhome` 会被 JS 踢回 `/`。

scheduler 手动验证：

```powershell
curl.exe http://localhost:9001/video-del-rec/test001
```

在 scheduler 终端应能看到一轮调度日志；`video_del_rec` 表里对应记录被清掉。

## 常见报错对照表

| 现象 | 原因 | 处理 |
|---|---|---|
| `go build` 报 `expected 'package', found 'EOF'` | web 目录空文件还在 | 检查是否执行了 `git rm -r web` |
| 浏览器控制台 `Failed to fetch` | 目标端口服务没起 | 确认 9000/8080 都 `go run` 着 |
| 控制台 CORS 报错 | streamsever 没挂 CorsMiddleware，或中间件没在 `Use` 里 | 检查第 4 步 |
| 注册/登录返回 `Request body is not correct.` | JSON 字段名不对 | 字段必须是 `user_name`、`pwd` |
| 返回 `Database ops failed.` | 表没建 / .env 密码错 | 重跑 schema.sql；确认从根目录启动 |
| 上传 500，scheduler/streamsever 日志 `no such file or directory` | 根目录 `videos/` 不存在 | `New-Item -ItemType Directory videos` |
| scheduler 终端报 SQL 1064 | 第 0.2 步的 `VALUES (?)` 没补 | 补上再重启 |
| `/userhome` 能打开但马上跳回 `/` | localStorage 里没有 session_id | 确认登录成功后再访问 |

## 下阶段可做（本次不用）

- 用 tsmain.go 里的 `Worker` 实现每 N 秒定时清理一轮。
- 视频删除接口（api 调 scheduler 登记 `video_del_rec`），并统一 DELETE 语义。
- 把之前聊的「统一错误结构 + Gin 错误中间件」落地到三个服务。
- CORS 从 `*` 收紧到精确 origin；session 过期后调用 `LoadSessionsFromDB` 恢复。

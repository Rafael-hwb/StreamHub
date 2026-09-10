# StreamHub 功能完善路线图（先做完功能，再做优化）

> 定位：本路线图**只补业务功能**，不做架构/安全/错误处理重构。
> 那些留到功能全通之后，再按 `docs/upgrade-guide.md` 逐步做。
>
> 起点基线（2026-09）：
> - 注册/登录可用（session 存内存 + DB，前端 localStorage）；
> - 上传能把文件落到根目录 `videos/`，但**没登记 video_info**；
> - 没有视频列表、没有观看/详情页、没有评论、没有删除；
> - 后端 dbops 里已有 `AddVideo/GetVideo/DeleteVideo/AddComment/ListComments` 等函数，只差串起来。
>
> 上传流程决策（方案 B）：**视频 id 由后端生成**——前端先调
> `POST /api/videos` 创建记录并拿到 id，再用这个 id 把文件传到 streamsever。
> 这样 `video_info.id` 与磁盘文件名天然一致，dbops 原版 `AddVideo`
> （内部用 utils.NewUUID 生成 id）一行都不用改。
>
> 代码风格约定：所有 Go 示例都照你现有的 GetVideo / AddCredential / ListComments
> 的完整写法（Prepare → 检查 err → defer Close → QueryRow/rows → ErrNoRows 分支）。
> 看清函数放哪个文件、顶部要补哪些 import，再整段敲进去。

## 功能目标（全部完成 = DoD）

1. 用户：注册、登录、登出（已有，补登出按钮）。
2. 上传：先登记（title + 作者）拿到后端生成的 id → 再以该 id 上传文件。
3. 展示：我的视频列表、视频详情/播放页。
4. 评论：观看页能看评论、登录用户能发评论。
5. 删除：作者能删自己的视频，文件异步被 scheduler 清理。
6. 首页：公开视频列表（"广场"），未登录也能逛。

每步独立可提交、可验证。**一步一 commit**。

---

## Step 0：给"当前用户"配一个查 ID 的入口

### 目的
`video_info.author_id` 和 `comments.author_id` 都是 users 表的整数 id，
但你的 session 里只存了 `login_name`。评论、归属校验、我的列表都离不开用户 id。

### 0.1 dbops 函数

加到 `api/dbops/api.go`，风格和 `GetCredential` 完全一样：

```go
func GetUserIDByName(loginName string) (int, error) {
	stmtOut, err := dbConnection.Prepare("SELECT id FROM users WHERE login_name = ?")
	if err != nil {
		return 0, err
	}
	defer stmtOut.Close()

	var id int
	err = stmtOut.QueryRow(loginName).Scan(&id)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}

	return id, nil
}
```

> 注意：dbops/api.go 顶部已经 import 了 `database/sql`，所以 `sql.ErrNoRows`
> 直接用。查不到用户时返回 `0, nil`，和 `GetCredential` 查不到返回空密码一致，
> 调用方自己判断 id == 0 的情况。

### 0.2 用法约定

受保护的 handler 里：先 `username := context.GetHeader(HEADER_FIELD_USERNAME)`
（SessionMiddleware 已注入），再 `dbops.GetUserIDByName(username)` 拿 id。
学习阶段每请求查一次库没问题。

### 验收

注册一个用户后，用 mysql 客户端确认 users 表有 id；调 `GetUserIDByName` 能查到。

---

## Step 1：上传（先登记拿 id → 再传文件）

### 目的
视频 id 由后端统一生成：前端先建记录拿到 id，再用它上传文件。
文件落盘名 = `video_info.id`，播放、删除、清理全部用同一个 id，不会对不上。

### 1.1 defs：请求体和错误码

请求体**只留 Title**——id 由后端生成，前端不传、也没有发言权：

```go
type VideoCreateRequest struct {
	Title string `json:"title"`
}
```

`api/defs/error.go` 按你现有的 var 块风格补两个错误（404 和 403 后面几步要用）：

```go
var (
	// ...原有四个不动...
	ErrorVideoNotFound = ErrResponse{HttpSC: 404, Error: Err{Error: "Video not found.", ErrorCode: "005"}}
	ErrorNotVideoOwner = ErrResponse{HttpSC: 403, Error: Err{Error: "Not the owner of this video.", ErrorCode: "006"}}
)
```

### 1.2 dbops：直接用原版 AddVideo

**不需要新增函数**。原版 `AddVideo(aid int, title string)` 内部会：

1. `utils.NewUUID()` 生成 id；
2. 插入 `video_info`（含 `display_ctime = time.Now()`）；
3. 返回带 `Id` 的 `*defs.VideoInfo`。

所以登记这一步完全复用它，不用改。

### 1.3 handler

加到 `api/handlers.go`（和 CreateUser 同一个文件）：

```go
func CreateVideoInfo(context *gin.Context) {
	username := context.GetHeader(HEADER_FIELD_USERNAME)
	aid, err := dbops.GetUserIDByName(username)
	if err != nil || aid == 0 {
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	videoBody := &defs.VideoCreateRequest{}
	if err := context.ShouldBindJSON(videoBody); err != nil {
		SendErrorResponse(context, defs.ErrorRequestBodyParseFailed)
		return
	}

	if len(videoBody.Title) == 0 {
		SendErrorResponse(context, defs.ErrorRequestBodyParseFailed)
		return
	}

	// 原版 AddVideo：内部生成 uuid 并插入 video_info，返回带 Id 的结果
	videoInfo, err := dbops.AddVideo(aid, videoBody.Title)
	if err != nil {
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	SendNormalResponse(context, 201, videoInfo)
}
```

> 命名约定：HTTP handler 用"动作 + 资源"（CreateVideoInfo，和代码库 CreateUser 同风格）；
> 请求体类型是"资源 + 动作 + Request"（VideoCreateRequest）；返回的是领域模型 VideoInfo。
> `defs.VideoInfo` 已经带小写 json tag（id / author_id / title / display_ctime），
> 所以前端读 `data.id` 这些小写键即可。

### 1.4 路由

`api/main.go`：把现在空壳的 `/api` 组改成两个组——**公开组** 和 **登录组**
（同一前缀注册不同 method+path 不冲突）。**只注册本步已实现的路由**，
后面的路由等对应 Step 写完再加，否则编译不过：

```go
publicAPI := router.Group("/api")
publicAPI.GET("/videos", ListVideos)                        // Step 7 再加
publicAPI.GET("/videos/:vid", GetVideoInfo)                // Step 3 再加
publicAPI.GET("/videos/:vid/comments", ListCommentsHandler) // Step 4 再加

authAPI := router.Group("/api")
authAPI.Use(SessionMiddleware)
authAPI.POST("/videos", CreateVideoInfo)                    // 本步
authAPI.GET("/my/videos", MyVideos)                         // Step 2 再加
authAPI.POST("/videos/:vid/comments", AddCommentHandler)    // Step 4 再加
authAPI.DELETE("/videos/:vid", DeleteVideo)                 // Step 6 再加
```

### 1.5 前端：userhome 上传改三步

userhome.html 加一个"视频标题"输入框，upload() 改成：

```js
async function upload() {
  const title = document.getElementById("title").value;
  const fileInput = document.getElementById("file-input");
  if (!title) { alert("请填写标题"); return; }
  if (!fileInput.files[0]) { alert("请先选择文件"); return; }
  const msg = document.getElementById("msg");

  // 1. 先登记：后端生成 id 并写入 video_info
  const metaRes = await fetch("/api/videos", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Session-Id": localStorage.getItem("session_id"),
    },
    body: JSON.stringify({ title }),
  });
  const meta = await metaRes.json();
  if (!metaRes.ok) {
    msg.textContent = meta.error || "登记失败";
    return;
  }

  const vid = meta.id;   // VideoInfo 已带小写 json tag，直接读 meta.id

  // 2. 用后端给的 id 上传文件（文件名 = vid）
  const formData = new FormData();
  formData.append("file", fileInput.files[0]);
  const res = await fetch("http://localhost:9000/upload/" + vid, {
    method: "POST",
    body: formData,
  });

  if (res.ok) {
    msg.textContent = "上传成功，视频 id：" + vid;
    const player = document.getElementById("player");
    player.src = "http://localhost:9000/videos/" + vid;
    player.hidden = false;
    loadMyVideos();   // Step 2 实现后调用
  } else {
    msg.textContent = "文件上传失败：" + res.status +
      "（video_info 里已有一条空记录，Step 6 的删除接口可用来清理）";
  }
}
```

> 方案 B 的已知代价：记录先建、文件后传，文件传失败会留一条"空记录"。
> 处理方式：学习阶段先提示即可；Step 6 实现 DELETE 后，失败分支补一句
> `fetch("/api/videos/" + vid, { method: "DELETE", headers: {...} })` 清掉它。

### 验收

1. 上传成功后：`video_info` 多一行，`videos/` 目录下多一个**同 id 命名**的文件；
2. 用 mysql 查到的 id 访问 `http://localhost:9000/videos/<id>` 能播；
3. 不填标题直接传 → 400，不产生记录。

---

## Step 2：视频列表 + "我的视频"

### 2.1 dbops 函数

加到 `api/dbops/api.go`，遍历风格照抄 `ListComments`：

```go
func ListVideosByAuthor(aid int) ([]*defs.VideoInfo, error) {
	var result []*defs.VideoInfo

	stmtOut, err := dbConnection.Prepare("SELECT id, title, display_ctime FROM video_info WHERE author_id = ?")
	if err != nil {
		return result, err
	}
	defer stmtOut.Close()

	rows, err := stmtOut.Query(aid)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	for rows.Next() {
		var id, title, ctime string
		if err := rows.Scan(&id, &title, &ctime); err != nil {
			return result, err
		}
		videoInfo := &defs.VideoInfo{Id: id, AuthorId: aid, Title: title, DisplayCtime: ctime}
		result = append(result, videoInfo)
	}

	if err := rows.Err(); err != nil {
		return result, err
	}

	return result, nil
}
```

### 2.2 handler

加到 `api/handlers.go`：

```go
func MyVideos(context *gin.Context) {
	username := context.GetHeader(HEADER_FIELD_USERNAME)
	aid, err := dbops.GetUserIDByName(username)
	if err != nil || aid == 0 {
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	videos, err := dbops.ListVideosByAuthor(aid)
	if err != nil {
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	SendNormalResponse(context, 200, videos)
}
```

注册到 `authAPI.GET("/my/videos", MyVideos)`。

### 2.3 userhome 渲染列表

页面加载（校验完登录态）后调 `/api/my/videos`，渲染成行：

```js
async function loadMyVideos() {
  const res = await fetch("/api/my/videos", {
    headers: { "X-Session-Id": localStorage.getItem("session_id") },
  });
  if (!res.ok) { location.href = "/"; return; }
  const videos = await res.json();
  // 每行：标题(videos[i].title) + display_ctime(videos[i].display_ctime)
  //       + "播放"按钮（跳 /video.html?vid=<videos[i].id>）
  //       + "删除"按钮（Step 6 接）
}
```

### 验收

上传 2-3 个视频后，userhome 刷新能看到列表；换一个用户登录，看不到别人的视频。

---

## Step 3：视频详情/播放页（新建 static/video.html）

### 3.1 dbops：详情（带作者名）

`api/defs/apidefs.go` 加（这个结构体**带 json tag**，是给前端的输出模型）：

```go
type VideoDetail struct {
	Id           string `json:"id"`
	AuthorId     int    `json:"author_id"`
	AuthorName   string `json:"author_name"`
	Title        string `json:"title"`
	DisplayCtime string `json:"display_ctime"`
}
```

加到 `api/dbops/api.go`：

```go
func GetVideoDetail(vid string) (*defs.VideoDetail, error) {
	stmtOut, err := dbConnection.Prepare(`SELECT video_info.id, video_info.author_id,
							video_info.title, video_info.display_ctime, users.login_name
						FROM video_info
						INNER JOIN users ON video_info.author_id = users.id
						WHERE video_info.id = ?`)
	if err != nil {
		return nil, err
	}
	defer stmtOut.Close()

	var (
		id         string
		aid        int
		title      string
		ctime      string
		authorName string
	)
	err = stmtOut.QueryRow(vid).Scan(&id, &aid, &title, &ctime, &authorName)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	detail := &defs.VideoDetail{Id: id, AuthorId: aid, AuthorName: authorName,
		Title: title, DisplayCtime: ctime}
	return detail, nil
}
```

### 3.2 handler

加到 `api/handlers.go`：

```go
func GetVideoInfo(context *gin.Context) {
	vid := context.Param("vid")

	video, err := dbops.GetVideoDetail(vid)
	if err != nil {
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}
	if video == nil {
		SendErrorResponse(context, defs.ErrorVideoNotFound)
		return
	}

	SendNormalResponse(context, 200, video)
}
```

注册到 `publicAPI.GET("/videos/:vid", GetVideoInfo)`。

> 注意路由参数名：api 侧你写 `:vid`，handler 里就是 `context.Param("vid")`；
> streamsever 9000 那个是 `:vid-id` / `Param("vid-id")`，两个服务别混。

### 3.3 页面

新建 `static/video.html`，注册到 api/main.go：

```go
router.StaticFile("/video.html", "./static/video.html")
```

页面逻辑：

```js
const params = new URLSearchParams(location.search);
const vid = params.get("vid");

// 1. fetch `/api/videos/${vid}` 拿 meta（VideoDetail 带小写 tag：data.title / data.author_name）
// 2. document.title = data.title
// 3. <video src="http://localhost:9000/videos/${vid}">
// 4. 评论区容器先留空（Step 4/5 填充）
```

没有登录也能看（公开）；评论框只在有 session 时显示。

### 验收

从 userhome 点"播放"跳到 `/video.html?vid=xxx`，能播、标题作者正确；
访问不存在的 vid 返回 404 而不是白屏。

---

## Step 4：评论后端（能写能查）

### 4.1 defs

`api/defs/apidefs.go` 加：

```go
type CommentCreateRequest struct {
	Content string `json:"content"`
}
```

### 4.2 dbops：简单评论查询

原版 `ListComments` 带时间范围参数，页面用不上，另加一个（加到 `api/dbops/api.go`）：

```go
func ListCommentsByVideo(vid string) ([]*defs.Comment, error) {
	var result []*defs.Comment

	stmtOut, err := dbConnection.Prepare(`SELECT comments.id, users.login_name, comments.content
						FROM comments
						INNER JOIN users ON comments.author_id = users.id
						WHERE comments.video_id = ?`)
	if err != nil {
		return result, err
	}
	defer stmtOut.Close()

	rows, err := stmtOut.Query(vid)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	for rows.Next() {
		var id, name, content string
		if err := rows.Scan(&id, &name, &content); err != nil {
			return result, err
		}
		result = append(result, &defs.Comment{Id: id, AuthorName: name, Content: content})
	}

	if err := rows.Err(); err != nil {
		return result, err
	}

	return result, nil
}
```

> 原版 ListComments 里 `VideoId: id` 那行其实是把评论 id 错放进了 VideoId，
> 这里我们不再重复那个错误。

### 4.3 handler

加到 `api/handlers.go`：

```go
func ListCommentsHandler(context *gin.Context) {
	vid := context.Param("vid")

	comments, err := dbops.ListCommentsByVideo(vid)
	if err != nil {
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	SendNormalResponse(context, 200, comments)
}

func AddCommentHandler(context *gin.Context) {
	vid := context.Param("vid")
	username := context.GetHeader(HEADER_FIELD_USERNAME)

	video, err := dbops.GetVideo(vid)
	if err != nil {
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}
	if video == nil {
		SendErrorResponse(context, defs.ErrorVideoNotFound)
		return
	}

	aid, err := dbops.GetUserIDByName(username)
	if err != nil || aid == 0 {
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	commentBody := &defs.CommentCreateRequest{}
	if err := context.ShouldBindJSON(commentBody); err != nil {
		SendErrorResponse(context, defs.ErrorRequestBodyParseFailed)
		return
	}
	if len(commentBody.Content) == 0 {
		SendErrorResponse(context, defs.ErrorRequestBodyParseFailed)
		return
	}

	err = dbops.AddComment(vid, aid, commentBody.Content)
	if err != nil {
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	SendNormalResponse(context, 201, gin.H{"success": true})
}
```

路由：

```go
publicAPI.GET("/videos/:vid/comments", ListCommentsHandler)
authAPI.POST("/videos/:vid/comments", AddCommentHandler)
```

### 验收（curl 阶段）

```powershell
curl.exe -H "X-Session-Id: <sid>" -X POST http://localhost:8080/api/videos/<vid>/comments -H "Content-Type: application/json" -d "{\"content\":\"hello\"}"
curl.exe http://localhost:8080/api/videos/<vid>/comments
```

第二条能看到刚发的评论和用户名；不带登录态的 POST 返回 401。

---

## Step 5：评论前端（video.html 里展示 + 发布）

### 5.1 展示

```js
async function loadComments() {
  const res = await fetch(`/api/videos/${vid}/comments`);
  const comments = await res.json();
  // 逐条渲染：作者名 + 内容
  // 安全红线：内容必须用 textContent 赋值，严禁 innerHTML 拼用户输入（防 XSS）
}
```

### 5.2 发布

有 session 时显示输入框；提交：

```js
const res = await fetch(`/api/videos/${vid}/comments`, {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "X-Session-Id": localStorage.getItem("session_id"),
  },
  body: JSON.stringify({ content }),
});
if (res.ok) { loadComments(); }  // 发完刷新列表
```

401 时提示重新登录并清 localStorage。

### 验收

浏览器里：登录用户能发评论，发完立刻显示；未登录看不到输入框；
评论里带 `<script>` 或 HTML 会**以纯文本显示**（不会执行）。

---

## Step 6：删除视频（后端闭环 + 前端按钮）

### 6.1 handler

加到 `api/handlers.go`（`net/http` 和 `log` 两个 import 需要补到文件顶部）：

```go
func DeleteVideo(context *gin.Context) {
	vid := context.Param("vid")
	username := context.GetHeader(HEADER_FIELD_USERNAME)

	aid, err := dbops.GetUserIDByName(username)
	if err != nil || aid == 0 {
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	video, err := dbops.GetVideo(vid)
	if err != nil {
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}
	if video == nil {
		SendErrorResponse(context, defs.ErrorVideoNotFound)
		return
	}
	if video.AuthorId != aid {
		SendErrorResponse(context, defs.ErrorNotVideoOwner)
		return
	}

	if err := dbops.DeleteVideo(vid); err != nil {
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	// 通知 scheduler 登记清理；失败只记日志，不影响主流程
	resp, err := http.Get("http://localhost:9001/video-del-rec/" + vid)
	if err != nil {
		log.Printf("Notify scheduler error: %v", err)
	} else {
		resp.Body.Close()
	}

	SendNormalResponse(context, 200, gin.H{"success": true})
}
```

注册到 `authAPI.DELETE("/videos/:vid", DeleteVideo)`。

> scheduler 的 `GET /video-del-rec/:vid-id` 会往 `video_del_rec` 插记录，
> taskrunner 周期执行时删文件 + 删记录。api 删的是业务记录，文件清理是异步的
> （scheduler 下一轮才执行）。方案 B 的"空记录"也可以借这个接口清理：
> 上传文件失败时，前端调 DELETE 会把刚建的记录删掉。

### 6.2 前端

userhome 列表每行加"删除"按钮：

```js
async function deleteVideo(vid) {
  if (!confirm("确定删除这个视频吗？")) return;
  const res = await fetch(`/api/videos/${vid}`, {
    method: "DELETE",
    headers: { "X-Session-Id": localStorage.getItem("session_id") },
  });
  if (res.ok) { loadMyVideos(); }  // 刷新列表
}
```

### 验收

1. 作者删除：video_info 消失；等 scheduler 一轮后根目录文件也没了，`video_del_rec` 无残留；
2. 非作者访问 DELETE 返回 403；
3. 删除不存在的 vid 返回 404；
4. scheduler 没启动时删除仍成功（文件等 scheduler 起来后补删）。

---

## Step 7：首页广场（公开视频列表）

### 7.1 dbops

加到 `api/dbops/api.go`：

```go
func ListAllVideos() ([]*defs.VideoDetail, error) {
	var result []*defs.VideoDetail

	stmtOut, err := dbConnection.Prepare(`SELECT video_info.id, video_info.author_id,
							video_info.title, video_info.display_ctime, users.login_name
						FROM video_info
						INNER JOIN users ON video_info.author_id = users.id`)
	if err != nil {
		return result, err
	}
	defer stmtOut.Close()

	rows, err := stmtOut.Query()
	if err != nil {
		return result, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id         string
			aid        int
			title      string
			ctime      string
			authorName string
		)
		if err := rows.Scan(&id, &aid, &title, &ctime, &authorName); err != nil {
			return result, err
		}
		result = append(result, &defs.VideoDetail{Id: id, AuthorId: aid, AuthorName: authorName,
			Title: title, DisplayCtime: ctime})
	}

	if err := rows.Err(); err != nil {
		return result, err
	}

	return result, nil
}
```

### 7.2 handler

加到 `api/handlers.go`：

```go
func ListVideos(context *gin.Context) {
	videos, err := dbops.ListAllVideos()
	if err != nil {
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	SendNormalResponse(context, 200, videos)
}
```

注册到 `publicAPI.GET("/videos", ListVideos)`。

### 7.3 前端

index.html 页面加载就 `fetch("/api/videos")`，登录框下方渲染视频卡片列表；
点卡片跳 `/video.html?vid=...`。有 session 时显示"进入用户主页"按钮，没有则显示登录框。

### 验收

未登录打开 `/`：能看到所有视频、能点进观看页、能看评论；登录后多出上传/我的入口。

---

## Step 8：收尾联调

按真实用户路径完整走一遍：

1. 注册新用户 A → 自动进 userhome；
2. A 上传 2 个视频（先登记拿 id → 再传文件，带标题）→ 列表出现、文件与记录同名；
3. A 在其中一个视频页发 2 条评论；
4. 退出登录 → 广场还能看视频和评论，但看不到评论框；
5. 注册用户 B 登录 → 广场点进 A 的视频 → 发评论成功；
6. B 对 A 的视频点删除 → 403；
7. A 登录删掉自己一个视频 → 列表消失，scheduler 日志显示文件被清；
8. 全程无 500、无白屏、无控制台报错。

全部通过 = 功能完善达成，再翻开 `docs/upgrade-guide.md` 从 Phase 0 开始做优化。

---

## 功能接口总览（完成后应有）

| 方法 | 路径 | 登录 | 用途 |
|---|---|---|---|
| POST | /user | 否 | 注册 |
| POST | /user/login | 否 | 登录 |
| POST | /api/videos | 是 | 创建视频记录并返回 id（随后用它上传文件） |
| GET | /api/videos | 否 | 视频广场 |
| GET | /api/my/videos | 是 | 我的视频 |
| GET | /api/videos/:vid | 否 | 视频详情 |
| GET | /api/videos/:vid/comments | 否 | 评论列表 |
| POST | /api/videos/:vid/comments | 是 | 发评论 |
| DELETE | /api/videos/:vid | 是 | 删自己的视频（含清空记录） |

## 需要新增的文件/改动汇总

| 文件 | 动作 |
|---|---|
| api/dbops/api.go | 加 5 个函数：GetUserIDByName / ListVideosByAuthor / GetVideoDetail / ListCommentsByVideo / ListAllVideos（AddVideo 原版复用，不改） |
| api/defs/apidefs.go | 加 VideoCreateRequest（只含 Title）/ VideoDetail / CommentCreateRequest |
| api/defs/error.go | 加 ErrorVideoNotFound(404) / ErrorNotVideoOwner(403) |
| api/handlers.go | 加 7 个 handler（CreateVideoInfo / MyVideos / GetVideoInfo / ListCommentsHandler / AddCommentHandler / DeleteVideo / ListVideos）；补 net/http、log import |
| api/main.go | 拆 publicAPI / authAPI 两组，注册新路由 + StaticFile video.html |
| static/userhome.html | 标题输入框、先登记再上传、列表渲染、删除按钮 |
| static/video.html | 新建：播放 + 详情 + 评论 |
| static/index.html | 广场列表渲染 |

## 与 upgrade-guide 的关系

本路线图刻意"先能用、后好看"：

- 评论内容用 `textContent` 防 XSS、删除做归属校验——这两条是功能必须，不算重构；
- 错误响应暂时沿用现在的 `SendErrorResponse` 风格，等 upgrade Phase 3 再统一信封；
- 视频 ID 白名单校验、密码 bcrypt、CORS 收紧等安全项，留在功能全通后的 upgrade Phase 1；
- handler 里"先查用户名再查 id"的写法，等 upgrade 时可换成 session 直接存 UserID，减少查询；
- json tag 命名已统一为小写下划线（id / author_id / display_ctime）；
  upgrade 阶段若做统一响应信封，再决定是否把键名改成 code/message/data 包装。

# 阶段 6：输入与路径安全 —— 阻断路径穿越

> 改造范围：`streamsever/handlers.go`、`scheduler/taskrunner/task.go`。
> 前置：阶段 5（会话生命周期）已提交。

## 1. 学习目标

理解「永不信任外部输入」。路径穿越（Path Traversal）是「外部输入直接拼进文件系统路径」的典型后果：攻击者用 `../` 逃出目标目录，读、写、删除任意文件。

## 2. 现状与问题

三处都是 `VIDEO_DIR + vid` 这类直接拼接：

| # | 位置 | 危险 |
| --- | --- | --- |
| 1 | [streamsever/handlers.go:28](streamsever/handlers.go:28) `os.Open(VIDEO_DIR + vid)` | `GET /videos/../.env` 可读任意文件 |
| 2 | [streamsever/handlers.go:55](streamsever/handlers.go:55) `SaveUploadedFile(file, VIDEO_DIR+vid)` | `POST /upload/../x` 可写任意位置 |
| 3 | [scheduler/taskrunner/task.go:13](scheduler/taskrunner/task.go:13) `os.Remove(VIDEO_PATH + vid)` | 删除记录若被污染可删任意文件 |

先自己写一个 `curl` 验证漏洞确实存在（读一个 `VIDEO_DIR` 之外的文件，例如 `../.env`），再动手修。亲眼看到漏洞被触发，比背「不要拼路径」记得牢。

## 3. 分步骤改法

### 步骤 1：加一个共享的 UUID 校验

`vid` 是服务端用 `utils.NewUUID()` 生成的，格式固定（8-4-4-4-12 的十六进制）。最稳的防御是**校验输入匹配 UUID 格式**，不匹配就拒绝，而不是事后「清洗路径」：

```go
// streamsever/validate.go
package main

import "regexp"

var uuidRe = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`,
)

func validVideoID(vid string) bool {
	return uuidRe.MatchString(vid)
}
```

> 为什么「校验 UUID」比「用 `filepath.Clean` 折叠 `../`」更好？因为业务上 `vid` 只能是 UUID，不符合就是非法请求，应该在最外层拒绝；而 `Clean` 只是把 `..` 折叠掉，等于「容忍了不该出现的输入」。两者结合最稳：先校验，再兜底。

### 步骤 2：在 `StreamHandler` 和 `UploadHandler` 入口校验

`streamsever/handlers.go` 的 `StreamHandler`：

```go
func StreamHandler(context *gin.Context) {
	vid := context.Param("vid-id")
	if !validVideoID(vid) {
		SendErrorResponse(context, ErrResponse{
			HttpCode: http.StatusBadRequest,
			Err: ErrStruct{ErrMessage: "Invalid video id.", ErrCode: "005"},
		})
		return
	}
	// 后续保持原逻辑
	videoLink := VIDEO_DIR + vid
	// ...
}
```

`UploadHandler` 同样在最前面加同一段校验。`TestPageHandler` 不需要（不碰文件路径）。

### 步骤 3：在 `DeleteVideo` 里校验（问题 3）

`scheduler/taskrunner/task.go` 的 `DeleteVideo`：

```go
func DeleteVideo(vid string) error {
	if !validVideoID(vid) {
		return fmt.Errorf("invalid video id %q", vid)
	}

	if err := os.Remove(VIDEO_PATH + vid); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
```

这里顺带修掉一个隐藏 bug：原代码 `os.Remove` 失败后只 `log.Printf` 然后返回 `nil`，把错误吞了。现在改成如实返回（除了「文件本来就不存在」这种可接受情况）。

> `scheduler` 的 `validVideoID` 需要自己定义一份（它和 `streamsever` 是不同的 `main` 包，不能互相导入）。这正是阶段 7 会解决「共享代码放哪」的信号——现在先各自定义，别急着抽公共包。

### 步骤 4：纵深防御 —— 路径兜底

作为第二道防线，加一个「最终路径必须仍在目标目录内」的检查，防止将来 `vid` 规则变化（比如允许用户自定义名字）时又出洞：

```go
import (
	"path/filepath"
	"strings"
)

func safePath(dir, name string) (string, error) {
	full := filepath.Join(dir, name)
	base := filepath.Clean(dir) + string(filepath.Separator)
	if !strings.HasPrefix(filepath.Clean(full), base) {
		return "", fmt.Errorf("path %q escapes base dir %q", name, dir)
	}
	return full, nil
}
```

在 `StreamHandler` 里：

```go
videoLink, err := safePath(VIDEO_DIR, vid)
if err != nil {
	SendErrorResponse(context, ErrorInternalFaults)
	return
}
video, err := os.Open(videoLink)
// ...
```

## 4. 验收标准

- [ ] `GET /videos/../.env` 返回 400（或 404），不泄露文件内容。
- [ ] `POST /upload/../x` 拒绝，不写文件。
- [ ] `DeleteVideo("../x")` 返回错误，不执行 `os.Remove`。
- [ ] 新增表驱动测试覆盖：合法 UUID、`../`、空串、超长串、非 UUID 字符串。
- [ ] `DeleteVideo` 不再吞 `os.Remove` 错误（除「文件不存在」）。
- [ ] 三件套通过。

## 5. 提交建议

```powershell
git add streamsever/handlers.go streamsever/validate.go scheduler/taskrunner/task.go
git commit -m "fix: block path traversal in stream and upload handlers"
```

## 6. 拓展思考题

1. 只用 `filepath.Clean` 而不做 UUID 校验，能防住路径穿越吗？`Clean` 处理不了哪些输入？
2. 校验放在「handler 入口」和放在「数据库层 / 文件层」有什么区别？为什么入口校验最优先？
3. `streamsever` 和 `scheduler` 都需要 `validVideoID`，现在各自复制一份，这算不算坏味道？什么时候该抽公共包，什么时候宁可复制？
4. 上传接口现在没有鉴权，任何知道地址的人都能传文件。这个问题本阶段为什么不解决？它属于后续哪个阶段的范畴？

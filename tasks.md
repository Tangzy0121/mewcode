# MewCode — Tasks(本轮任务拆解)

> 角色:回答"按什么顺序做、每步动哪些文件"。每个任务可在一次专注会话内完成。
> 推进时逐个勾选;依赖关系决定顺序;最后两个任务固定为"接入主流程"+"端到端验证"。
> 参考资料定位以 plan.md 的「各单元实现细节」对应小节为准。

---

- [x] **T1 — 项目骨架与配置** ✅ 2026-06-01
  - 做:`go mod init mewcode`,建 `internal/` 八个子包空壳(config/llm/conversation/agent/tools/permission/tui);实现 config(读环境变量 + flag、缺凭证报错、三档模式校验、默认值)。
  - 影响文件:`go.mod`、`main.go`(占位)、`internal/config/`、各包 `doc.go`、`.gitignore`
  - 依赖:无
  - 参考:plan.md「config」;默认值见 checklist C1
  - 完成判据:`go build ./...` 通过;缺凭证时运行打印指定提示。
  - **实测**:build/vet 通过;`go test ./internal/config/` 全绿(4 测);缺 key→stderr 提示含 ANTHROPIC_API_KEY + exit 1;有 key→ready 行;`--model/--mode` 覆盖生效;`--mode yolo`→报错 exit 1。

- [x] **T2 — conversation 消息模型 + llm 接口/事件/错误** ✅ 2026-06-01
  - 做:`internal/conversation` 定义后端无关的消息/内容块类型与 Manager(Add*/GetMessages 深拷贝/Len);`internal/llm` 定义 Client 接口(输入历史+工具 schema,输出事件流+错误 channel)、事件 sum type(TextDelta/ToolCallStart/Delta/Complete/StreamEnd + UsageInfo)、5 类分层错误、NewClient 工厂(按 protocol 路由)。
  - 影响文件:`internal/conversation/conversation.go(+_test)`、`internal/llm/{events,errors,client}.go(+_test)`
  - 依赖:T1
  - 参考:plan.md「provider(→llm)」前两段、「conversation」;ch02 对照微调
  - 完成判据:类型、接口、事件、错误编译通过,暂无后端实现。
  - **取舍**:序列化不放 conversation(教程放在 Manager 按 protocol 分发),改由各 provider 读 GetMessages 自译 → conversation 零后端依赖。Thinking 事件/字段、AddSystemReminder、MaxTokensSetter、model resolver 本轮全砍。
  - **实测**:build/vet 通过;`go test ./internal/conversation/ ./internal/llm/` 全绿(深拷贝隔离、errors.As 分类、未知 protocol、事件封口断言)。

- [x] **T3 — 六个工具 + 注册表** ✅ 2026-06-02
  - 做:实现 read / write / edit / glob / grep / bash 六个工具,每个含名称、描述、入参 Schema、execute;实现注册表(生成工具定义列表 + 按名派发)。bash 走 Windows PowerShell 且带超时;grep 优先 rg 回退遍历。
  - 影响文件:`internal/tools/`(tool.go 接口 + objectSchema/strProp helper、registry.go、read/write/edit/glob/grep/bash.go、tools_test.go)
  - 依赖:T2(用到工具定义类型)
  - 参考:plan.md「tools」;错误转 `is_error` 见 checklist C5
  - 完成判据:每个工具有临时目录单测且通过(见 checklist C5)。
  - **取舍**:`Execute(ctx, json.RawMessage)→(string,bool)`,出参直接对齐 conversation.ToolResultBlock(Content+IsError)。注册表 Definitions() 产出 `{name,description,input_schema}` 的 `[]map[string]any`,直接喂 Stream,llm 包零依赖 tools(教程常把工具定义类型耦合进 provider)。bash 超时做成可配(NewBashTool(timeout)),既给默认 30s 又让测试 300ms 快速验超时路径。glob 用 stdlib filepath.Glob(不支持 `**`,描述里讲明),不引 doublestar 外部依赖。grep 双路径:`rg` 在则调之(exit 1=无命中非错误),否则 WalkDir 回退、跳 .git、封顶 200 条。**read 三重封顶**(默认 2000 行 / 256KB / 单行 2000B)+ offset/limit 行范围,超限截断附续读提示——喂 LLM 的读工具不设上限会撑爆上下文,是底线不是可选;bash 输出同理封 64KB。(初版我漏了 read 上限、误当"MVP 可省",经 User 纠正补齐。)
  - **实测**:build/vet 通过;`go test ./internal/tools/` 全绿(8 测,含注册表长度=6/未知名派发/读缺失/写自动建目录/edit 不唯一与缺失不误改/glob/grep 命中/bash 跑通+超时)。全量 `go test ./...` 不回归。

- [x] **T4 — 三档权限判定** ✅ 2026-06-02
  - 做:实现三档模式枚举与判定函数(副作用工具 + 当前模式 → 放行/拒绝/需批准);定义"待决请求"与"用户决定"两个数据结构;实现运行中切换模式。
  - 影响文件:`internal/permission/permission.go(+_test)`;`internal/tools/`(Tool 接口加 `SideEffecting()` + 6 工具实现)
  - 依赖:T1
  - 参考:plan.md「permission」
  - 完成判据:三档 × (只读工具/副作用工具) 组合的判定单测通过(见 checklist C4)。
  - **取舍**:"是否有副作用"放 Tool 接口由各工具自报(read/glob/grep=false,write/edit/bash=true),**不在 permission 硬编码工具名清单**——避免清单与实际工具漂移(正是 [[feedback-mvp-not-an-excuse]] 那类隐患的预防)。Manager 持 mode + 本会话 alwaysAllow 集;"本会话总是允许"按工具名粒度生效。三态 Allow/Ask/Deny;plan 拒绝时给 `PlanDenialHint` 文本提示切档。
  - **实测**:build/vet 通过;`go test ./internal/permission/` 全绿(三档×只读/副作用矩阵、always-allow 按工具粘住且不影响其他工具、Apply 三选项、运行中切档改变判定)。

- [x] **T5 — fake provider 替身** ✅ 2026-06-02
  - 做:实现按预设脚本吐事件的假后端(文本→工具调用→收到结果后结束),供离线测试 agent loop。
  - 影响文件:`internal/llm/fake.go(+_test)`
  - 依赖:T2
  - 参考:plan.md「provider · fake 实现」
  - 完成判据:能脚本化驱动一次"先说话再调一次工具再结束"的事件流。
  - **取舍**:FakeClient 持 `[]FakeTurn`,每次 Stream 回放下一 turn(loop 一轮一调,多轮对话=turn 切片);turn 可带 Err 走错误路径。超额调用(脚本耗尽)回 LLMError 而非挂起/静默结束——暴露误用。编译期 `var _ Client = (*FakeClient)(nil)` 钉死接口一致。
  - **实测**:build/vet 通过;`go test ./internal/llm/` 全绿(脚本化"文本→工具→结束"、多轮推进、耗尽报错)。

- [x] **T6 — agent loop(用 fake 测通)** ✅ 2026-06-02
  - 做:实现主循环:追加历史→请求后端→消费事件流→遇工具调用先过 permission 判定→执行或返回被拒结果→回灌历史→直到本轮正常结束;持有内存历史。先全部用 channel 对外通信,用 T5 的 fake 驱动跑通。
  - 影响文件:`internal/agent/agent.go(+_test)`;`internal/tools/registry.go`(加 `Get`)
  - 依赖:T2、T3、T4、T5
  - 参考:plan.md「agent loop」
  - 完成判据:fake 驱动的全链路测试通过(说话→调工具→收结果→结束,且权限被拒路径也覆盖,见 checklist C6)。
  - **取舍**:对 UI 全经 channel——`Events()` 出(AssistantText/ToolStarted/ToolFinished/PermissionAsked/TurnFinished/ErrorOccurred 封口 sum type),`SubmitDecision`/`SetMode` 入;loop 不碰 UI 状态(满足解耦)。RunTurn 同步跑一轮(调用方起 goroutine),Ask 时阻塞 `<-decisions`。续轮判据用"是否有 toolCalls"而非 StopReason,更稳。错误(errc)→发 ErrorOccurred 后结束本轮,agent 不崩、仍接下一句。系统提示归 client(NewClient 时注入),agent 不存死字段(与 plan 措辞小异)。
  - **实测**:build/vet 通过;`go test ./internal/agent/`(及 `-race`)全绿——happy path(历史 user/assistant+tooluse/tool_result/assistant 四条且内容正确)、权限被拒续跑(is_error 被拒结果)、工具出错续跑。

- [x] **T7 — Anthropic provider 落地** ✅ 2026-06-02
  - 做:实现真实后端:POST Messages 端点带 stream,逐行解析 SSE,把 tool_use 入参增量拼接后整体解析,翻译成 T2 的统一事件流;把统一类型双向翻译为 Anthropic 报文。
  - 影响文件:`internal/llm/anthropic.go(+_test)`、`client.go`(NewClient 接真后端)、`llm_test.go`(去 stub 断言)
  - 依赖:T2
  - 参考:plan.md「provider · Anthropic 实现」;线格式经 WebFetch 官方 streaming 文档核对
  - 完成判据:用真实 key 手动跑一次,能流式取回文本与一次工具调用(见 checklist C2)。
  - **路线**:User 拍板**手写 net/http + SSE**(不用官方 SDK,贴合 spec「造轮子学协议」)。无 client 级超时(流是长连接),取消靠 ctx。请求只发 model/max_tokens/system/messages/tools/stream——Opus 4.8 禁的 temperature/top_p/budget_tokens 一律不带。SSE 按 index 维护 tool_use 块,input_json_delta 累积到 content_block_stop 整体出 ToolCallComplete。错误分 HTTP 状态(401/403→Auth,429→RateLimit+retry-after,413→ContextTooLong)和流内 error 事件两路。
  - **重试(2026-06-02 补)**:`send` 对 429/500/502/503/529/网络错做**指数退避重试**(最多 3 次,首退 500ms 上限 8s,429 尊重 Retry-After);4xx(auth/400)不重试;流中途的 error 事件不重试(已吐部分输出)。退避基数做成字段供测试压到 1ms。
  - **实测**:build/vet 通过;`go test ./internal/llm/` 全绿——httptest 模拟真实 SSE 流,验证文本逐字拼接、tool_use 跨 3 个 partial_json 增量重组为合法 JSON `{"location": "San Francisco, CA"}`、usage 取值、请求体含 `stream:true`+system、401→AuthenticationError 分类、**503 两次→第三次成功(3 次尝试)、401 不重试(1 次)**。**真 key 端到端留 T11/C8。**

- [x] **T8 — TUI:输入框 + 流式对话区** ✅ 2026-06-02
  - 做:bubbletea Model 持有对话区/输入框;Update 处理敲键、回车提交、文本增量;View 渲染可滚动对话区 + 底部输入框;用 bubbletea 命令把"等下一个 loop 事件"包成异步消息。
  - 影响文件:`internal/tui/model.go`、`render.go`(+`tui_test.go`);`internal/agent/agent.go`(加 `Start`);go.mod/sum(bubbletea+bubbles+lipgloss)
  - 依赖:T6
  - 参考:plan.md「tui」前半
  - 完成判据:接 fake provider 时,输入一句能看到流式逐字回复(见 checklist C7)。
  - **取舍**:UI 经 `Backend` 接口(Events/Start/SubmitDecision/SetMode)与 loop 通信,不共享可变状态;`waitForEvent` Cmd 每收一个事件就重挂,持续消费 channel。textinput 用 bubbles;对话区用"手动 tail 末 N 行"而非 viewport——少一层复杂度,demo 够用。`/mode`、`/quit` 走输入框命令。
  - **实测**:build/vet 通过;`go test ./internal/tui/` 全绿(流式逐字、Enter 起轮+busy、收尾提交);真 binary `--fake` 启动渲染正常(alt-screen+输入框+footer),无 panic。

- [x] **T9 — TUI:工具卡片 + 权限弹选** ✅ 2026-06-02
  - 做:在 Model 加入工具卡片列表与待决权限请求;Update 处理工具状态变化、权限请求(进弹选态)、权限选择回传 loop;View 渲染卡片与底部权限弹选(与输入框二选一)。
  - 影响文件:`internal/tui/model.go`、`render.go`(+`tui_test.go`)(与 T8 同批实现)
  - 依赖:T8
  - 参考:plan.md「tui」后半、「permission」
  - 完成判据:fake 驱动一次副作用工具调用时,卡片状态流转可见,权限弹选可操作(见 checklist C7)。
  - **取舍**:卡片按 ID 索引,lipgloss 圆角边框 + 颜色随状态(running 黄⏳ / ok 绿✓ / error 红✗);权限弹选占底部取代输入框,键 `a/s/d`(或 `1/2/3`)对应 允许一次/总是允许/拒绝。被拒/被 deny 若无 Started 事件则补建卡片。
  - **实测**:`go test ./internal/tui/` 全绿(卡片 running→ok 翻转、权限提示出现+按键回传 AllowOnce、弹选时隐藏输入 footer)。

- [x] **T10 — 接入主流程(入口装配)** ✅ 2026-06-02
  - 做:在 `main.go` 解析 flag → 加载 config → 构造 provider(真 Anthropic)、tools、permission、agent loop → 构造 TUI 并接上 loop → 启动 bubbletea。把前面各单元真正串成一个可执行程序。
  - 影响文件:`main.go`(加 systemPrompt 常量;非 fake 走 `llm.NewClient`,fake 走 demoClient,二者共用 agent+TUI 装配)
  - 依赖:T7、T9
  - 参考:plan.md「入口(main)」
  - 完成判据:`go build` 出 `mewcode`,启动进入 TUI,可对真实后端发起对话。
  - **实测**:`go build -o mewcode.exe` 成功;非 fake + 无 key→stderr 报缺 ANTHROPIC_API_KEY、exit 1;非 fake + 有 key→进 TUI 渲染不崩(无头超时杀)。真后端真聊天的逐条走查 = T11/C8(需你的真 key + 真终端)。

- [ ] **T11 — 端到端验证**
  - 做:按 checklist.md 的 C8 端到端剧本,用真实后端逐项走查"读代码问答 / 改代码(default 弹确认)/ 跑命令 / 三档切换 / 各类失败不崩溃 / 退出不留会话",逐条勾选。
  - 影响文件:无(可能据验证回头修补,记录在 checklist)
  - 依赖:T10
  - 参考:checklist.md 全部条目,重点 C8
  - 完成判据:checklist.md 全部勾完,方下"本轮完成"结论。

- [x] **T12 — OpenAI 兼容后端(可接 DeepSeek)** ✅ 2026-06-02 ← 追加
  - 做:实现 `protocol=openai` 的真实后端:POST `{base_url}/chat/completions` 带 `Authorization: Bearer`、`stream:true`;解析 OpenAI 风格 SSE(`data:{choices[].delta.content / .tool_calls[]}` + `data:[DONE]`),tool_calls 按 `index` 累积 `arguments` 后整体出 ToolCallComplete;把统一历史译为 OpenAI messages(system 首条、assistant 带 tool_calls、tool 结果用 `role:"tool"`+`tool_call_id` 逐条)。复用 retry/错误分类。
  - 影响文件:`internal/llm/openai.go(+_test)`、`client.go`(NewClient openai 分支)、`config.go`(openai 缺 base-url 时默认 DeepSeek)
  - 依赖:T2(接口)、T7(参照手写 SSE 范式 + 复用 retry/错误分类)
  - 参考:DeepSeek 官方(WebFetch + context7 核准):OpenAI 兼容、base `https://api.deepseek.com`、模型 `deepseek-chat`/`deepseek-reasoner`、delta 含 `reasoning_content`(本轮忽略)
  - 完成判据:httptest 离线验证 text+tool_calls 增量重组、错误分类;真 DeepSeek key 手动跑一次可流式取回文本与一次工具调用(见 checklist C9)。
  - **取舍**:base-url 缺省时默认 DeepSeek(`https://api.deepseek.com`),`--base-url` 可改投别的 OpenAI 兼容服务;reasoning_content(reasoner 思考)忽略只取 content;OpenAI 无 is_error 标志 → 失败结果在 tool 消息内容前缀 `ERROR:`;tool_calls 按 index 累积 args、首块捕获 id/name。retry/错误分类复用 T7(`classifyHTTPError`/`isRetryableStatus`/`backoffDelay`,后者从 anthropic 抽成包级 free func)。
  - **实测**:build/vet 通过;`go test ./internal/llm/` 全绿——httptest 模拟真实 OpenAI 流,验文本逐块拼接、tool_calls 跨 3 个 delta 增量重组为 `{"location": "SF"}`、Bearer 头、`stream:true`+system+tools 翻译、usage、role:tool 历史翻译。**真 DeepSeek key 端到端并入 C8/C9 真机走查。**

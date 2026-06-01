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

- [ ] **T3 — 六个工具 + 注册表**
  - 做:实现 read / write / edit / glob / grep / bash 六个工具,每个含名称、描述、入参 Schema、execute;实现注册表(生成工具定义列表 + 按名派发)。bash 走 Windows PowerShell 且带超时;grep 优先 rg 回退遍历。
  - 影响文件:`internal/tools/`
  - 依赖:T2(用到工具定义类型)
  - 参考:plan.md「tools」;错误转 `is_error` 见 checklist C5
  - 完成判据:每个工具有临时目录单测且通过(见 checklist C5)。

- [ ] **T4 — 三档权限判定**
  - 做:实现三档模式枚举与判定函数(副作用工具 + 当前模式 → 放行/拒绝/需批准);定义"待决请求"与"用户决定"两个数据结构;实现运行中切换模式。
  - 影响文件:`internal/permission/`
  - 依赖:T1
  - 参考:plan.md「permission」
  - 完成判据:三档 × (只读工具/副作用工具) 组合的判定单测通过(见 checklist C4)。

- [ ] **T5 — fake provider 替身**
  - 做:实现按预设脚本吐事件的假后端(文本→工具调用→收到结果后结束),供离线测试 agent loop。
  - 影响文件:`internal/llm/`(fake 文件)
  - 依赖:T2
  - 参考:plan.md「provider · fake 实现」
  - 完成判据:能脚本化驱动一次"先说话再调一次工具再结束"的事件流。

- [ ] **T6 — agent loop(用 fake 测通)**
  - 做:实现主循环:追加历史→请求后端→消费事件流→遇工具调用先过 permission 判定→执行或返回被拒结果→回灌历史→直到本轮正常结束;持有内存历史。先全部用 channel 对外通信,用 T5 的 fake 驱动跑通。
  - 影响文件:`internal/agent/`
  - 依赖:T2、T3、T4、T5
  - 参考:plan.md「agent loop」
  - 完成判据:fake 驱动的全链路测试通过(说话→调工具→收结果→结束,且权限被拒路径也覆盖,见 checklist C6)。

- [ ] **T7 — Anthropic provider 落地**
  - 做:实现真实后端:POST Messages 端点带 stream,逐行解析 SSE,把 tool_use 入参增量拼接后整体解析,翻译成 T2 的统一事件流;把统一类型双向翻译为 Anthropic 报文。
  - 影响文件:`internal/llm/`(anthropic 文件)
  - 依赖:T2
  - 参考:plan.md「provider · Anthropic 实现」
  - 完成判据:用真实 key 手动跑一次,能流式取回文本与一次工具调用(见 checklist C2)。

- [ ] **T8 — TUI:输入框 + 流式对话区**
  - 做:bubbletea Model 持有对话区/输入框;Update 处理敲键、回车提交、文本增量;View 渲染可滚动对话区 + 底部输入框;用 bubbletea 命令把"等下一个 loop 事件"包成异步消息。
  - 影响文件:`internal/tui/`
  - 依赖:T6
  - 参考:plan.md「tui」前半
  - 完成判据:接 fake provider 时,输入一句能看到流式逐字回复(见 checklist C7)。

- [ ] **T9 — TUI:工具卡片 + 权限弹选**
  - 做:在 Model 加入工具卡片列表与待决权限请求;Update 处理工具状态变化(运行中/成功/失败更新卡片)、权限请求(进弹选态)、权限选择回传 loop;View 渲染卡片与底部权限弹选(与输入框二选一)。
  - 影响文件:`internal/tui/`
  - 依赖:T8
  - 参考:plan.md「tui」后半、「permission」
  - 完成判据:fake 驱动一次副作用工具调用时,卡片状态流转可见,权限弹选可操作(见 checklist C7)。

- [ ] **T10 — 接入主流程(入口装配)**
  - 做:在 `main.go` 解析 flag → 加载 config → 构造 provider(真 Anthropic)、tools、permission、agent loop → 构造 TUI 并接上 loop → 启动 bubbletea。把前面各单元真正串成一个可执行程序。
  - 影响文件:`main.go`
  - 依赖:T7、T9
  - 参考:plan.md「入口(main)」
  - 完成判据:`go build` 出 `mewcode`,启动进入 TUI,可对真实后端发起对话。

- [ ] **T11 — 端到端验证**
  - 做:按 checklist.md 的 C8 端到端剧本,用真实后端逐项走查"读代码问答 / 改代码(default 弹确认)/ 跑命令 / 三档切换 / 各类失败不崩溃 / 退出不留会话",逐条勾选。
  - 影响文件:无(可能据验证回头修补,记录在 checklist)
  - 依赖:T10
  - 参考:checklist.md 全部条目,重点 C8
  - 完成判据:checklist.md 全部勾完,方下"本轮完成"结论。

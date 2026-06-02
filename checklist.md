# MewCode — Checklist(本轮验收清单)

> 角色:每一项必须可勾选、可观测。不许写"实现完整""质量良好"。
> spec 里被砍掉的具体值(默认值、错误文本、阈值)在此作为验收项落地。
> 至少一条端到端验收(见 C8)。全部勾完才下"本轮完成"结论。

---

## C1 — 配置(config)

- [x] 凭证环境变量名为 `ANTHROPIC_API_KEY`;`grep -rn "ANTHROPIC_API_KEY" internal/config` 返回 ≥1 条。 ✅ T1
- [x] 默认模型为 `claude-opus-4-8`;不传 flag/env 时,config 解析出的 Model 等于该值(单测 TestLoadDefaults)。请求体核对留 T7。 ✅ T1(config 层)
- [x] 默认权限模式为 `default`;不传 `--mode` 时 config.Mode = default(单测 TestLoadDefaults)。界面显示核对留 T10。 ✅ T1(config 层)
- [x] 缺失 `ANTHROPIC_API_KEY` 启动时,程序退出且 stderr 打印含 "ANTHROPIC_API_KEY" 字样的提示,退出码非 0。 ✅ T1(实测 exit=1)
- [x] `--model` 与 `--mode` flag 可覆盖默认值:传入后 config 反映新值(单测 TestLoadFlagOverride + 实测 ready 行)。界面/请求体核对留 T7/T10。 ✅ T1(config 层)

## C2 — provider(Anthropic)

- [x] 请求带 `stream: true`;httptest 断言请求体含 `"stream":true`(+system)。 ✅ T7(TestAnthropicStreamParsesTextAndToolUse;真请求抓包待 C8)
- [x] 输入一句纯问答(不触发工具),界面能看到回复**逐字**出现,而非一次性整段。 ✅ T7 代码层(text_delta→TextDelta 逐块;界面逐字视觉待 C8 真机)
- [x] 输入一句必然触发读文件的请求,事件流中出现一次工具调用,其入参是合法 JSON。 ✅ T7 代码层(tool_use→ToolCallComplete,入参解析为合法 JSON;真模型触发待 C8)
- [x] tool_use 入参跨多个 SSE 增量时,拼接后整体解析成功(用一个入参较长的工具调用验证)。 ✅ T7(TestAnthropicStreamParsesTextAndToolUse:3 个 partial_json 增量→`{"location": "San Francisco, CA"}`)

## C3 — llm 接口抽象

- [x] 存在后端无关的接口与领域类型;`grep -rn "interface" internal/llm internal/conversation` 命中 `Client`/`StreamEvent` 接口定义。 ✅ T2
- [x] 5 类分层错误齐全且可 `errors.As` 分类(单测 TestErrorsAreClassifiable);事件 sum type 经 `streamEvent()` 私有方法封口(编译期断言 `var _ = []StreamEvent{...}`)。 ✅ T2
- [x] fake 与 anthropic 两个实现都满足同一接口(注释或编译期断言可证),agent loop 不直接 import 具体后端类型。 ✅ T7(两侧均有编译期断言 `var _ Client = (*FakeClient)(nil)` / `var _ Client = (*anthropicClient)(nil)`;agent 只依赖 llm.Client 接口)

## C4 — 权限(三档)

- [x] 三档模式取值为 `plan` / `default` / `auto`;判定经 config.ModePlan/ModeDefault/ModeAuto 三常量。 ✅ T4
- [x] plan 模式下,对写/改/执行工具的判定结果为"拒绝",且提示用户切档(`PlanDenialHint`)。 ✅ T4(TestDecideMatrix/TestSetModeAtRuntime)
- [x] default 模式下,对只读工具(读/glob/grep)判定为"放行",对写/改/执行判定为"需批准"。 ✅ T4(TestDecideMatrix;只读由 Tool.SideEffecting()=false 驱动)
- [x] auto 模式下,对所有工具判定为"放行"。 ✅ T4(TestDecideMatrix)
- [x] 上述组合有单测覆盖;`go test ./internal/permission/` 通过。 ✅ T4(4 测全绿)

## C5 — 工具(6 个)

- [x] 注册表生成的工具定义列表长度为 6,名称含 read / write / edit / bash / glob / grep 对应工具。 ✅ T3(TestDefaultRegistryHasSixNamedTools)
- [x] 读文件:对不存在的路径返回标记为出错(`is_error`)的结果而非 panic。 ✅ T3(TestReadMissingPath)
- [x] 读文件:支持 offset/limit 行范围;读大文件时受三重封顶(默认 2000 行 / 256KB / 单行 2000B)保护,超限截断并附"如何续读"提示,不一次性灌爆上下文。 ✅ T3(TestReadRangeAndCaps)
- [x] 执行命令:输出受 64KB 字节上限保护,超长截断并提示。 ✅ T3(TestBashRunAndTimeout 末段)
- [x] 写文件:写到不存在的父目录时能自动建目录并写成功。 ✅ T3(TestWriteCreatesParentDirs)
- [x] 改文件:当待替换旧文本在文件中不唯一或不存在时,返回出错结果(供模型纠正),不误改。 ✅ T3(TestEditUniqueness,含"两次失败后文件未变"断言)
- [x] 执行命令:在 Windows 下经 PowerShell 运行,能取回 stdout/stderr 与退出码;超长命令受超时上限保护(超时返回出错而非挂死)。 ✅ T3(TestBashRunAndTimeout)
- [x] grep:系统有 `rg` 时调用之,无 `rg` 时回退遍历仍能返回命中。 ✅ T3(TestGrepFindsMatch;代码双路径,rg 分支 exit 1=无命中非错误,回退分支 WalkDir 跳 .git 封顶 200)
- [x] 各工具临时目录单测通过;`go test ./internal/tools/` 通过。 ✅ T3(8 测全绿)

## C6 — agent loop(fake 驱动)

- [x] fake 脚本"先文本→调一次工具→收结果→结束"能被主循环完整跑通,内存历史最终包含 user/assistant/tool_result 各内容块。 ✅ T6(TestLoopRunsToolAndRecordsHistory,历史 4 条且 tool_result 内容=文件内容)
- [x] 权限被拒路径:fake 触发一个副作用工具且模拟用户拒绝时,历史中出现一条"被拒"性质的 tool_result,循环继续而非中断。 ✅ T6(TestLoopContinuesAfterRefusal)
- [x] 工具执行出错路径:工具返回 `is_error` 时,结果照常回灌,循环继续。 ✅ T6(TestLoopContinuesAfterToolError)
- [x] `go test ./internal/agent/` 通过(含 `-race`)。 ✅ T6

## C7 — TUI

- [x] 接 fake provider 时,输入一句能看到流式逐字回复出现在对话区。 ✅ T8(TestStreamingTextAppears;demo 后端逐字 35ms 流式;逐字"视觉"待真机 `--fake` 确认)
- [x] 工具调用在界面上以独立卡片呈现,且状态随执行从"运行中"变为"成功"或"失败"。 ✅ T9(TestToolCardStatusFlips:⏳→✓)
- [x] default 模式触发副作用工具时,底部出现可选择的权限确认项(允许一次 / 本会话总是允许 / 拒绝),选择后循环据此继续。 ✅ T9(TestPermissionPromptAndChoice:a/s/d→AllowOnce 回传)
- [x] 界面与主循环不共享可变状态:经 `Backend` 接口 + `eventMsg`/`waitForEvent` Cmd + agent 的 Events/decisions channel 通信。 ✅ T8/T9
- [x] 对话区可上滚看历史(viewport):内容溢出时新输出自动跟随底部,PgUp/PgDn/鼠标滚轮可回看,上滚阅读时不被新输出拽下。 ✅ 补做(TestViewportScrollsThroughHistory)

## C8 — 端到端验收(真实后端,本轮总验收)

> 用真实 `ANTHROPIC_API_KEY` 启动 `mewcode`,逐条走查并勾选。

- [ ] **读问答**:问"这个项目的 main 做了什么",模型调读/搜工具后流式给出基于真实文件内容的回答。
- [ ] **改代码**:让它在某文件加一行注释;default 模式下弹出确认,批准后该文件确实新增了该行(`grep` 可验)。
- [ ] **跑命令**:让它运行 `go version`;权限放行后命令执行,输出回灌,模型据此回应。
- [ ] **三档切换**:启动选 plan,让它改文件→被拒并提示;切到 default→可批准;切到 auto→不再弹确认直接执行。
- [ ] **失败不崩溃**:故意断网或用错 key 发一次请求,界面显示错误消息而程序不退出;让它读不存在的文件,模型收到出错结果后自行改道。
- [ ] **退出不留会话**:退出后重启,上一轮对话历史不再存在(本轮预期行为)。
- [x] `go build ./...` 与 `go vet ./...` 均无报错。 ✅ T7/T10(全 8 包 build/vet 净、`go test ./...` 全绿)

> 上面 6 条**读问答/改代码/跑命令/三档切换/失败不崩溃/退出不留会话**需你用真 `ANTHROPIC_API_KEY` 在真终端 `mewcode`(不带 `--fake`)逐条走查后勾选——代码与装配已就绪(T1–T10 全绿),这是本轮唯一剩下的「真机实测」。

## C9 — OpenAI 兼容后端(DeepSeek)

- [x] `protocol=openai` 时请求打 `{base_url}/chat/completions`,带 `Authorization: Bearer`、body 含 `"stream":true`;httptest 断言之。 ✅ T12(TestOpenAIStreamParsesTextAndToolCall)
- [x] OpenAI 风格 SSE 文本逐块:`choices[].delta.content` → 逐字 TextDelta;`data:[DONE]` 收尾。 ✅ T12
- [x] tool_calls 跨多个 delta 增量(按 `index` 累积 `arguments`、id/name 首块捕获)→ 拼回合法 JSON,出一次 ToolCallComplete。 ✅ T12(`{"location": "SF"}`)
- [x] 历史译为 OpenAI 格式:system 首条、assistant 带 `tool_calls`、工具结果用 `role:"tool"`+`tool_call_id` 逐条。 ✅ T12(TestBuildOpenAIMessagesTranslatesToolFlow)
- [x] 非 2xx 错误分类(401→Auth、429→RateLimit)+ 瞬时错误重试(复用 T7 retry/分类:`classifyHTTPError`/`isRetryableStatus`/`backoffDelay`)。 ✅ T12(复用 T7 已测路径)
- [x] `go test ./internal/llm/` 全绿(上述 httptest 离线验证)。 ✅ T12
- [ ] **真机**:用真 DeepSeek key 跑 `mewcode --protocol openai --base-url https://api.deepseek.com --model deepseek-chat`,能流式问答 + 调一次工具(并入 C8 真机走查)。

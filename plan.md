# MewCode — Plan(本轮实现计划)

> 角色:回答"具体怎么做、用什么技术、实现细节、最终做成什么样"。比 spec 更落地,是 tasks.md 拆解的依据。
> 与 spec 的关系:spec 定"做什么/边界",plan 定"怎么做";spec 不写的默认值/类型设计,这里写。

## 技术选型

| 关注点 | 选型 | 理由 |
| ------ | ---- | ---- |
| 语言 | Go 1.22+ | 单二进制、并发原语好、跨平台 |
| TUI | `bubbletea` + `lipgloss` | Go 生态事实标准,Elm 架构天然契合"消息驱动、状态不可变" |
| HTTP/流式 | 标准库 `net/http` + 手写 SSE 解析 | Anthropic 流式走 SSE,无需重 SDK,便于看清协议 |
| JSON | 标准库 `encoding/json` | 工具入参/出参、消息体均为 JSON |
| 搜索底层 | 优先调用系统 `rg`(ripgrep),缺失时回退标准库遍历 | 快且省事,回退保证可用 |
| 测试 | 标准库 `testing` + 临时目录 | 工具用临时目录隔离;后端用 fake 替身 |

## 目录结构(最终形态)

```
MewCode/
├── go.mod
├── main.go                  # 入口:装配并启动
├── spec.md / plan.md / tasks.md / checklist.md
└── internal/
    ├── config/              # 配置加载
    ├── llm/                 # 模型后端接口 + Anthropic 实现 + fake 替身(原名 provider)
    ├── conversation/        # 两层消息模型:Manager 持历史, 按 protocol 序列化
    ├── agent/               # 智能体主循环(历史委托给 conversation.Manager)
    ├── tools/               # 工具注册表 + 6 个工具
    ├── permission/          # 三档模式 + 判定
    └── tui/                 # bubbletea 模型与视图
```

> **ch02 对照微调(2026-06-01)**:参照教程"取精华去糟粕"后调整——
> - 采纳:`provider` 改名 `llm`(与教程一致便于对照);拆出独立 `conversation` 包(两层消息模型,序列化收口);错误分 5 类(Auth/RateLimit/Network/ContextTooLong/通用)放 llm 层;idle 超时兜底 SDK 静默阻塞。
> - 不取(本轮):Extended Thinking/Adaptive 思考、reasoning summary、thinking signature 往返、OpenAI 落地、system-reminder 注入、model 短名 resolver。
> - 待定:Anthropic 走官方 SDK(教程做法)还是手写 net/http+SSE(原 plan),T7 再拍。

## 各单元实现细节

### config
- 从环境变量读取:模型凭证、模型名、后端 base URL;启动参数(flag)可覆盖模型名与权限模式。
- 缺凭证时:启动即失败并打印一行明确提示(具体文案见 checklist)。
- 默认值:模型 `claude-opus-4-8`;权限模式 `default`;凭证环境变量名 `ANTHROPIC_API_KEY`。

### provider(本轮核心难点之一)
- 定义后端接口:输入是"系统提示 + 消息历史 + 可用工具定义",输出是一个**事件流**(channel),事件类型涵盖:文本增量、工具调用开始/入参增量/结束、本轮结束原因、错误。
- 定义领域类型:消息(role + 内容块列表)、内容块(文本 / 工具调用 / 工具结果)、工具定义(名称 + 描述 + JSON Schema 入参)。这些类型与具体后端无关,Anthropic 实现负责双向翻译。
- Anthropic 实现:POST Messages 端点,带 `stream: true`,逐行解析 SSE 事件,翻译成上面的统一事件流。tool-use 块的入参以增量 JSON 拼接,块结束时整体解析。
- fake 实现:按预设脚本吐事件(如"先吐一段文本→再吐一个对某工具的调用→收到结果后吐结束"),供 agent loop 离线测试。

### tools
- 工具接口:名称、给模型看的描述、入参 JSON Schema、执行函数(入参 JSON → 结果文本 + 是否出错)。
- 注册表:把 6 个工具收进一个表,既能生成给后端的工具定义列表,又能按名派发执行。
- 六个工具:
  - 读文件:按路径读,可选行范围;文件不存在等错误转成 `is_error` 结果。
  - 写文件:整体覆盖或新建;必要时建父目录。
  - 改文件:在文件中把一段精确文本替换为另一段;旧文本不唯一或找不到则报错(交模型纠正)。
  - 执行命令:Windows 下经 PowerShell 运行;捕获 stdout/stderr 与退出码;设超时上限。
  - glob:按文件名模式匹配并返回路径列表。
  - grep:按正则在文件内容中搜索,优先 `rg`,回退遍历;返回命中位置。
- 副作用工具(写/改/执行)在 execute 前必须经 permission 放行——但放行**不是工具自己调**,而是 agent loop 在派发前调,保证统一不可绕过。

### permission
- 三档模式:plan(只读——任何写/执行一律拒绝并提示切档)/ default(读放行,写/执行需逐次批准)/ auto(全放行)。
- 判定函数:输入(工具名是否有副作用、当前模式)→ 输出三态之一:放行 / 拒绝 / 需用户批准。
- "需用户批准"时,产出一个待决请求对象,由 agent loop 转发给 TUI;TUI 收集用户选择(允许一次 / 本会话总是允许 / 拒绝)回传。
- 运行中切换模式:通过界面指令更新当前模式。

### agent loop
- 持有:系统提示、内存中的消息历史、对 provider/tools/permission 的引用。
- 单轮流程:追加 user 消息 → 调 provider 取事件流 → 消费:文本增量转成界面消息;遇工具调用,先 permission 判定(需批准则发请求并阻塞等待界面回传决定),据决定执行或返回"被拒"结果;把工具结果作为新内容块追加历史 → 若本轮结束原因是"还要调工具"则继续请求后端,直到"正常结束"。
- 与界面解耦:loop 在独立 goroutine 运行,所有对界面的输出与对界面的等待都经 channel / bubbletea 消息,不直接操作界面状态。

### tui(本轮核心难点之二)
- bubbletea Model 持有:对话区内容(含流式中的当前回复)、输入框、工具卡片列表、待决权限请求、当前权限模式。
- Update 处理的消息类型:用户敲键(交给输入框)、回车提交(把输入发给 agent loop)、来自 loop 的文本增量(追加到当前回复)、工具状态变化(更新对应卡片)、权限请求(进入弹选态)、权限选择结果(回传 loop)、错误(作为系统消息显示)。
- View:对话区(可滚动)+ 工具卡片 + 输入框/权限弹选(二选一占据底部)。
- 接缝:用 bubbletea 的命令机制把"等待 loop 的下一个事件"包装成异步命令,事件到达即作为消息进入 Update。

### 入口(main)
- 解析 flag(权限模式、模型名覆盖)→ 加载 config(失败即退出)→ 构造 provider、tools 注册表、permission、agent loop → 构造 TUI Model 并把 loop 接上 → 启动 bubbletea 程序。

## 实现顺序(为什么这样排)

先底层后上层、先可测后难测:**config → provider 类型/接口 → tools → permission → fake provider → agent loop(用 fake 测通)→ Anthropic provider(真后端)→ TUI → 入口装配 → 端到端验证**。
这样每完成一层都能独立验证,TUI 与真实网络这两个最难测的环节留到地基稳固后再接。

## 最终做成什么样

一个 `mewcode` 可执行文件:终端启动后是一个 TUI,顶部可滚动对话区、底部输入框;输入请求后模型流式作答,需要动手时弹出带颜色的工具卡片和权限确认;读/搜可直接进行,写/执行按档把关;任何错误都被温和地显示而非崩溃。代码按 `internal/` 七单元组织,新增工具或后端只需实现接口并注册。

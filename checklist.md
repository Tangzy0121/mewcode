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

- [ ] 请求带 `stream: true`;抓一次真实请求,body 中 `grep '"stream"'` 命中 true。
- [ ] 输入一句纯问答(不触发工具),界面能看到回复**逐字**出现,而非一次性整段。
- [ ] 输入一句必然触发读文件的请求,事件流中出现一次工具调用,其入参是合法 JSON。
- [ ] tool_use 入参跨多个 SSE 增量时,拼接后整体解析成功(用一个入参较长的工具调用验证)。

## C3 — llm 接口抽象

- [x] 存在后端无关的接口与领域类型;`grep -rn "interface" internal/llm internal/conversation` 命中 `Client`/`StreamEvent` 接口定义。 ✅ T2
- [x] 5 类分层错误齐全且可 `errors.As` 分类(单测 TestErrorsAreClassifiable);事件 sum type 经 `streamEvent()` 私有方法封口(编译期断言 `var _ = []StreamEvent{...}`)。 ✅ T2
- [ ] fake 与 anthropic 两个实现都满足同一接口(注释或编译期断言可证),agent loop 不直接 import 具体后端类型。 ⏳ 待 T5/T7

## C4 — 权限(三档)

- [ ] 三档模式取值为 `plan` / `default` / `auto`;`grep -rn "plan\|default\|auto" internal/permission` 命中三者。
- [ ] plan 模式下,对写/改/执行工具的判定结果为"拒绝",且提示用户切档。
- [ ] default 模式下,对只读工具(读/glob/grep)判定为"放行",对写/改/执行判定为"需批准"。
- [ ] auto 模式下,对所有工具判定为"放行"。
- [ ] 上述组合有单测覆盖;`go test ./internal/permission/` 通过。

## C5 — 工具(6 个)

- [ ] 注册表生成的工具定义列表长度为 6,名称含 read / write / edit / bash / glob / grep 对应工具。
- [ ] 读文件:对不存在的路径返回标记为出错(`is_error`)的结果而非 panic。
- [ ] 写文件:写到不存在的父目录时能自动建目录并写成功。
- [ ] 改文件:当待替换旧文本在文件中不唯一或不存在时,返回出错结果(供模型纠正),不误改。
- [ ] 执行命令:在 Windows 下经 PowerShell 运行,能取回 stdout/stderr 与退出码;超长命令受超时上限保护(超时返回出错而非挂死)。
- [ ] grep:系统有 `rg` 时调用之,无 `rg` 时回退遍历仍能返回命中。
- [ ] 各工具临时目录单测通过;`go test ./internal/tools/` 通过。

## C6 — agent loop(fake 驱动)

- [ ] fake 脚本"先文本→调一次工具→收结果→结束"能被主循环完整跑通,内存历史最终包含 user/assistant/tool_result 各内容块。
- [ ] 权限被拒路径:fake 触发一个副作用工具且模拟用户拒绝时,历史中出现一条"被拒"性质的 tool_result,循环继续而非中断。
- [ ] 工具执行出错路径:工具返回 `is_error` 时,结果照常回灌,循环继续。
- [ ] `go test ./internal/agent/` 通过。

## C7 — TUI

- [ ] 接 fake provider 时,输入一句能看到流式逐字回复出现在对话区。
- [ ] 工具调用在界面上以独立卡片呈现,且状态随执行从"运行中"变为"成功"或"失败"。
- [ ] default 模式触发副作用工具时,底部出现可选择的权限确认项(允许一次 / 本会话总是允许 / 拒绝),选择后循环据此继续。
- [ ] 界面与主循环不共享可变状态:`grep -rn "go func\|chan \|Msg" internal/tui internal/agent` 显示二者经 channel/Msg 通信。

## C8 — 端到端验收(真实后端,本轮总验收)

> 用真实 `ANTHROPIC_API_KEY` 启动 `mewcode`,逐条走查并勾选。

- [ ] **读问答**:问"这个项目的 main 做了什么",模型调读/搜工具后流式给出基于真实文件内容的回答。
- [ ] **改代码**:让它在某文件加一行注释;default 模式下弹出确认,批准后该文件确实新增了该行(`grep` 可验)。
- [ ] **跑命令**:让它运行 `go version`;权限放行后命令执行,输出回灌,模型据此回应。
- [ ] **三档切换**:启动选 plan,让它改文件→被拒并提示;切到 default→可批准;切到 auto→不再弹确认直接执行。
- [ ] **失败不崩溃**:故意断网或用错 key 发一次请求,界面显示错误消息而程序不退出;让它读不存在的文件,模型收到出错结果后自行改道。
- [ ] **退出不留会话**:退出后重启,上一轮对话历史不再存在(本轮预期行为)。
- [ ] `go build ./...` 与 `go vet ./...` 均无报错。

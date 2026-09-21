# 统一后处理配置——强一致扩展（冻结存档 v10）

> Status: **FROZEN ARCHIVE v10（2026-09-20 范围收口"砍到核心"时整体冻结）——非现行设计**
> 本文件=设计文档 v10 全文原样存档。多写者并发/编排器集成/SDK 强重试语义等需求出现时，作为演进设计启用并解冻评审；在此之前以 `unified-postprocess-config-design.md`（v11 核心）为准。
> 冻结范围：oplog/op_id 客户端契约/r1-r2 双域指纹/p1 patch 编码/T2 全状态机与补偿/reconcile 循环两子相位/corrupt 隔离+recovery-resync/GetPostprocessOperation+202-200-409 形态/capability token/双实例号/EPHEMERAL 模式。
> 以下为 v10 原文：

> Status: **design draft v10 — 按第九轮评审（1 P1 + 1 P2 + 四点裁决）全量修订（2026-09-20），未批准实施**
> 依据：提案 v2；官方 postprocess 库 1.12.1；09-20 裸输出链 93.72 实测；锚点核验累计（ai-runtime gRPC `grpc::InsecureServerCredentials()` main.cpp:171 / post run 要求 ROI 附着 TensorPriv :1439-1445 / backend_function 于 infer create 前注入 platform_config model_manager.cpp:330-353 / native_yolov8_pose create 期开关 :1162 / ModelRegisterRequest 现役至 17、ModelRegisterResponse 现役仅 1-2）。
> v9→v10 变更摘要：
> ① **CanonicalPostprocessPatch 独立编码（#1）**：patch 不复用完整文档 canonicalizer——完整文档侧会填默认值，**omitted ≠ 显式清空(空列表) ≠ 显式设默认值** 三态会被折叠，两种执行效果不同的 delta 可能同指纹被误判幂等重试。新增 `p1:` 域独立编码：presence 规则（在档键=显式编码，含空列表；omitted=不产生字节）+ 独立版本前缀；别名折叠/数值规范/ParamValue 编码与完整文档**共享实现**；fingerprint **双域分离**：runtime 环=完整文档域 `r1:`、平台 oplog=patch 域 `r2:`——两作用域指纹不可直接比较；
> ② **corrupt 拒写后移（#2）**：U0 收窄为纯静态鉴权+请求外形（**"不触 DB 数据行"改"不产生 DB 写入"**）；corrupt 检查移至 **U0' 未命中后的新操作门**——已有 op 重试仍可返回原终态（触发隔离的 op 返回 `failed(DB_INVARIANT_VIOLATION)` 而非通用拒写）；U0' 本身只读；
> ③ **corrupt 恢复面=REST admin 唯一入口（裁决 1）**：新增 §3.5 `recovery-resync`——platform-api REST admin endpoint（**不新增运维向 ai-runtime 管理 gRPC**——config_state/revisions/active/oplog 全在 platform-api/DB，直连 runtime 只能重装内存态无法原子解除 DB 隔离；普通 ApplyMode **不设穿透参数**，runtime 侧只用既有 token 保护 RECONCILE）；流程=独立 admin RBAC+锁 → corrupt→**recovering** → `reload_active`（校验 active）/`restore_revision`（**复制旧 revision 为新最高代，不倒退 active 指针**——append-only 保持）→ oplog `RECOVERY_RESYNC` → 内部 RECONCILE 安装核对 → **完全核对成功才 healthy**；config_state 增 `recovering`；
> ④ **canonical 等价重试表述修正（裁决 2）**：性质准确表述为"**规范化后语义等价**"（非文本相同）；依赖 legacy 翻译与 U0' 共享同一规范化实现——列为实现期不变量测试；
> ⑤ **op_id 客户端契约定稿（裁决 3）**："重试必须保持规范化请求语义及全部并发前置条件不变；rebase 或任何语义修改都必须使用新的 op_id"；KEY_REUSED 标准文案；SDK 区分 `retry(existingOperation)`/`apply(newOperation)`（后者始终生成新 UUID）；
> ⑥ **HTTP 形态定稿（裁决 4）**：pending|compensating→`202 Accepted`+`Location: /operations/{op_id}`；任一终态→`200 OK`；KEY_REUSED→`409 Conflict`；`Retry-After` 仅在服务端能给出稳定轮询建议时返回；SDK 对 200/202 同一解析结构。

---

## 0. 设计总览

```
Web 向导 │ REST │ SDK(gRPC) │ AMPK 包          ← 四入口，全部提交统一文档
    ▼
┌─ platform-api / app-manager（Go）────────────────────────────────┐
│ 组装文档；schema 校验；DB append-only revisions + active 指针       │
│ （全 CAS + 唯一约束；基线=请求固定 base 的 revision）；migration     │
│ （旧行 gen=1 基线）；oplog（reason/legacy_no_cas/r2 指纹）；         │
│ reconcile（全量扫描+两子相位+装载协同）；GetPostprocessOperation；  │
│ corrupt 隔离与 recovery-resync（REST admin，§3.5）；指标；          │
│ P1 期过渡合成器（P3 退役）                                          │
└──────────────────────────────────────────────────────────────────┘
    ▼  gRPC ApplyPostprocessDocument(mode, doc, base, target, op_id)   [capability token]
┌─ ai-runtime（C++）───────────────────────────────────────────────┐
│ 注册：三条路径状态机（fresh/alias/co-owner，§2；实例号二分）        │
│ 更新：按模型串行 → 实例捕获 → 幂等环 → mode 前置 → 快路径          │
│       → 三层翻译 → 新建会话 → 验证 → 锁内实例 CAS + 快照 swap      │
│ 直连：EPHEMERAL only（epoch+revision 成对强制）；RuntimeState 查询 │
│ 旧 UpdatePostprocessConfig：服务端适配器→ legacy_no_cas（审计）    │
└──────────────────────────────────────────────────────────────────┘
    ▼
HAL（零改动基线；CLIP readiness 修复为 P3 优先项，§3.1 缺陷表）
```

六条设计红线：

- R1 接受≠消费：每键 `effect` 标注，`advisory` 键 UI 必带不生效提示；
- R2 热更逐键 `mutability`；**`model_type`/`decoder` 为注册身份字段，值变化一律拒绝**；`create_only/requires_reload` 显式拒绝（仅拒**值变化**的键）；
- R3 张量契约校验在真实 model_info 存在后执行（两阶段），错配注册时拒绝；
- R4 raw 量化元数据是 P4 前置；
- R5 **append-only + 持久化先行 + hash 收敛**：DB revisions 只增不改（唯一约束），active 指针**全部推进均为带期望代的 CAS**；desired 先于任何应用落 DB；收敛判定=canonical hash 相等（§1.4）；失败按 typed 结果分类——仅证明未应用者可补偿（补偿本身带 active-CAS），结果未知者交给 reconcile 终态清算（§3.4A）；**无隐式 rebase**（desired 只能基于请求固定 base 合成）；
- R6 **会话级原子 + 生命周期安全**：swap 不依赖 HAL 原子性；提交点=锁内**实例 CAS + 单次 noexcept 快照交换**；旧会话由最后在飞快照释放销毁；**无任何 destroy-then-create 回退**。

## 1. 核心数据结构

### 1.1 统一配置文档与 apply 协议（proto，可编译示意）

```protobuf
// inference.proto 增量（proto3，已核验）。新 message 字段号自 1 起；
// ModelRegisterRequest 增项挂 20+（现至 17=batch_size）；
// ModelRegisterResponse 增项挂 3+（现役仅 model_id=1/status=2）。
message StringList  { repeated string items = 1; }
message FloatList   { repeated double items = 1; }
message FloatMatrix { repeated FloatList rows = 1; }

message ParamValue {
  oneof kind {
    string      s      = 1;
    double      d      = 2;
    bool        b      = 3;
    StringList  list   = 4;   // labels / prompts / negative_prompts
    FloatList   flist  = 5;
    FloatMatrix matrix = 6;   // anchors[][]（Q5 开放，载体先备）
  }
}
message ParamEntry { string key = 1; ParamValue value = 2; }

message PostprocessDocument {
  string model_type      = 1;
  string decoder         = 2;   // "raw" 一等；单解码器类型可省略
  repeated ParamEntry params = 3;   // 重复键=校验拒绝
  string legacy_original = 4;   // 原文逐字节——仅审计，永不参与合成与 hash
  string legacy_residual = 5;   // 安全管线产出——合成可消费，参与 canonical hash（§1.4）
  string legacy_source   = 6;   // db_old_row | ampk_old | keypoint_grandfather（不参与 hash）
}

enum ApplyMode {
  APPLY_MODE_UNSPECIFIED = 0;  // 入口与客户端拒绝
  PERSISTENT_CAS = 1;          // 平台控制面（token）：base>0、target>current
  RECONCILE      = 2;          // 平台控制面（token）：单向安装
  EPHEMERAL      = 3;          // 外部直连：overlay，epoch+revision 成对强制（U0）
}
enum ApplyResult {
  APPLY_RESULT_UNSPECIFIED = 0;  // 客户端/入口拒绝
  APPLIED                  = 1;
  IDEMPOTENT               = 2;  // (op_id,完整指纹) 已应用——UNKNOWN 重试安全网
  REJECTED_SAFE            = 3;  // 校验/构造/验证失败——证明未应用，唯一常规可补偿类
  BASE_CONFLICT            = 4;  // 世代/epoch+revision/实例/base 已变，本请求未应用（不补偿）
  STALE                    = 5;  // target 过期（已有后继）→ superseded
  MODE_DENIED              = 6;  // 无权使用该 mode（U0 预检）
  IDEMPOTENCY_KEY_REUSED   = 7;  // 同 op_id 异指纹——证明未应用（处置见 §3.3）
}

message ApplyPostprocessDocumentRequest {
  string              model_id = 1;
  PostprocessDocument doc      = 2;
  ApplyMode           mode     = 3;
  uint64 base_generation       = 4;  // PERSISTENT_CAS 必填>0；U2 基线=该代 revision（§3.3）
  uint64 target_generation     = 5;  // PERSISTENT_CAS 必填>current；RECONCILE=DB active；EPHEMERAL 忽略
  optional uint64 expected_runtime_epoch    = 6;  // EPHEMERAL 必携（成对，U0 强制）；其余 mode 不携
  optional uint64 expected_runtime_revision = 7;  // 同上——新 RPC 无缺省豁免（legacy 见 §3.2）
  string op_id                 = 8;  // 1..128，[A-Za-z0-9._:-]；legacy 入口=服务端 UUID v4
}
message ApplyPostprocessDocumentResponse {
  ApplyResult result             = 1;
  string      message            = 2;
  uint64      applied_generation = 3;
  uint64      runtime_epoch      = 4;  // 返回新 epoch/revision（客户端下次 CAS 基线）
  uint64      runtime_revision   = 5;
  string      applied_doc_hash   = 6;  // "c1:<hex>"
  string      canonical_version  = 7;
}

// 运行态查询（gRPC 面；持久 op 终态不在此——ai-runtime 无 oplog）
message GetPostprocessRuntimeStateRequest { string model_id = 1; }
message GetPostprocessRuntimeStateResponse {
  string      applied_doc_hash     = 1;
  string      canonical_version    = 2;
  uint64      applied_generation   = 3;
  uint64      runtime_epoch        = 4;
  uint64      runtime_revision     = 5;
  string      last_op_id           = 6;
  ApplyResult last_op_result       = 7;
  ApplyMode   apply_mode           = 8;   // 当前态来源模式
  bool        overlay              = 9;   // EPHEMERAL 覆盖在途（重启即 false，§3.4 条款）
  uint64      overlay_base_generation = 10;  // overlay 前持久世代
  string      overlay_base_hash    = 11;     // overlay 前文档 hash
  uint64      infer_instance_id    = 12;     // 当前 inference 会话实例（apply CAS 主对象）
  uint64      entry_instance_id    = 13;     // 当前 registry 条目实例
}

// 注册增量（wire 完整）：
message ModelRegisterResponse {          // 增项（现役仅 1-2）
  // string model_id = 1;  Status status = 2;   ← 原样保留
  uint64 infer_instance_id = 3;          // 本次注册涉及/创建的 inference 会话实例
  uint64 entry_instance_id = 4;          // 本条目实例（alias 独立、co-owner 不变）
}
// ModelRegisterRequest 挂：
//   PostprocessDocument postprocess_doc = 20;  // 缺省=走旧 model_type/model_variant 翻译
//   uint64 initial_generation            = 21;  // >0 仅平台注册路径（token）可用；其余服务端覆写 0
//   string initial_doc_hash              = 22;  // 平台核对（canonical hash）
// GetPostprocessOperation 属平台管理面：platform-api REST，DB oplog 为源——见 §3.3。
```

- 重复键语义：`params` 同键两次=拒绝（§3.2 SAX）。
- `decoder:"raw"`：params/legacy 恒空。
- **mode 鉴权**：现状 ai-runtime 监听为 `grpc::InsecureServerCredentials()`（main.cpp:171，已核验）——ServerContext 不暴露 accept fd，**SO_PEERCRED 当下不可实现**。因此 **PERSISTENT/RECONCILE 强制 capability token**：本地配置文件（0600、ai-runtime 服务用户独占、platform-api 持有副本）+ 轮换 + 双令牌过渡窗口；经 gRPC metadata 传递，缺失/错误 → MODE_DENIED。**后续增强（非依赖）**：迁移 gRPC local credentials 或引入读取 Unix peer credential 的本地代理，落地后再叠加进程身份校验。外部 SDK 客户端仅 EPHEMERAL。
- **op_id 契约**：幂等环与 DB 均以 **op_id 为查找键**，命中后比较**完整指纹**（见下）——同 op_id 换 target/mode/hash 一律 KEY_REUSED。长度 1..128、字符集 [A-Za-z0-9._:-]，违者 U0 拒；legacy 入口不接收客户端 op_id，由服务端生成 UUID v4（不可预测 → 平台命名空间无被抢占面）。新 SDK `register/apply` 两入口同规则。
- **op_id 客户端契约（⑤）**：**重试必须保持规范化请求语义及全部并发前置条件（base/target/expected 对）不变**——键序与空白不影响（§1.4 规范化折叠）；**rebase 或任何语义修改都必须使用新的 op_id**。KEY_REUSED 标准文案：
  > 此 op_id 已绑定到另一请求。若要重试，请保持原请求及 base 不变；若已基于新状态重新提交，请生成新的 op_id。
  SDK 区分 `retry(existingOperation)`（按原语义重发）与 `apply(newOperation)`（始终生成新 UUID）。
- **fingerprint（① 双域分离）**：公式同构、domain 分离——
  - **runtime 环（完整文档域）**：`r1 = "r1:" + hex(SHA-256(model_id, mode, base, target, expected_epoch, expected_revision, c1_doc_digest))`——对 gRPC 所收**完整 doc** 计算（apply 步 1）；
  - **平台 oplog（patch 域）**：`r2 = "r2:" + hex(SHA-256(model_id, mode, base, target, expected_epoch, expected_revision, p1_patch_digest))`——对 REST 提交的 **patch** 计算（U0'，p1 编码见 §1.4，**不触 base/active**）；
  - 两作用域指纹**不可直接比较**（domain tag 不同）；分别存 DB oplog 行 / runtime 幂等环槽；客户端不传。外显：GetPostprocessOperation/审计=R2 格式、runtime 环=R1 格式；UI 示前 12 位。
- **epoch 语义**：`runtime_epoch` = 每次进程启动**随机生成的 nonce**（**仅等值比较语义、无序**——不用于判断新旧；64-bit 随机碰撞概率可忽略）；EPHEMERAL 的 CAS 基线=(epoch, revision) **成对**匹配。
- **无 CAS 豁免边界**：**新结构化 RPC 无任何缺省豁免**——EPHEMERAL 缺 expected 对即 U0 拒。无条件写仅存在于**旧 `UpdatePostprocessConfig` RPC 的服务端适配器**（显式 `legacy_no_cas` 内部路径，§3.2），逐次审计+指标计数；**sunset=该路径用量连续一个发布周期（或指定天数，配置项）为零后删除**——兼容窗口可拖延但边界可删。

### 1.2 schema 单源（typed 约束 + 验证分级）

`platform/platform-api/model/postprocess_schema.go`（Constraint/ParamDef/TensorExpect/DecoderSpec 同 v3，新增 ValidateTier）：

```go
type ValidateTier string // constructive | synthetic_smoke | reload_only

type DecoderSpec struct {
    // ...（v3 字段不变：ModelTypes/Decoder/Source/FixedName/Params/Hardcoded/
    //      Contract[NamePattern,Dims,CountMin/Max/Mod,Dtype,IsNMS]/Synthetic）
    Validate ValidateTier   // 判据=能否在"新建第二会话+验证"交换协议内确认可用
    Fixture  string         // synthetic_smoke 的 FixtureBuilder 标识（§3.1 第 7 步）
}
```

**初始定级**（P3 设备探测定稿，未探测者保守 reload_only）：

| decoder | ValidateTier | 依据 |
|---|---|---|
| detection 参数化/固定名（NMS 面） | `synthetic_smoke` | FixtureBuilder 可复刻 NMS 元数据张量；零张量→count=0→rc==0 |
| `native_yolov8_pose` | `synthetic_smoke` | 同上（保守） |
| CLIP 分类 | `reload_only`（保级前置：**HAL create readiness 修复[缺陷 #2/#4]——P3 优先 HAL 项** + FixtureBuilder 验证 → 升 synthetic_smoke） | create 忽略 init 失败；prompts 今天可走 apply 热路径——见兼容声明 |
| 其余（含 OCR 等当前可 apply 热更者） | `reload_only`（默认） | P3 探测逐 family 裁决 |

**兼容声明**：tier 判据是**交换协议内可验证性**。既有 apply 热路径键（CLIP prompts、OCR `det_*` 等）在 family 过保级前置前被拒热更（requires_reload）——**明示 breaking change**（§6、发版说明）+ reload 工作流（unload→按 DB active reload）。**P3 优先修复 HAL readiness 消除回归**。P3 探测枚举"当前可 apply 热更"family 清单逐个裁决，防静默回归。

`ValidateTier == reload_only` ⇒ 全键 mutability 降级 `requires_reload`——热更禁用，无回退（R6）。消费链：`go:generate` JSON + 构建期 generator 产 `postprocess_schema.h`（含 ValidateTier/Fixture）；CI 重生成 diff。

### 1.3 状态与世代

```
platform.db（持久，Go 拥有；append-only + 指针；全量唯一约束）
  postprocess_revisions: model_id, generation(单调), doc, kind(normal|compensation),
                         confirmed, created_at
                         **UNIQUE(model_id, generation)**——并发插入在 DB 层互斥
  model 行 active_generation 指针——**一切推进均为 CAS：UPDATE ... WHERE active_generation=<期望>**
  postprocess_oplog: op_id, model_id, target_generation, request_fingerprint(r2:),
                     status(pending|compensating|committed|rolled_back|failed|superseded),
                     reason(null|MODEL_DELETED|DB_INVARIANT_VIOLATION|RECOVERY_RESYNC|...),
                     legacy_no_cas(0|1),
                     detail, created_at, updated_at
                     **UNIQUE(model_id, op_id)**——U0' 比对既有行指纹
                     【实例号永不入 oplog——runtime 域标识，不参与 op 终态判定】
                     【legacy_no_cas 列=sunset 判据的持久观测面（"用量连续一个发布
                       周期为零"由该列按时间窗聚合——跨重启可查，不依赖 journal 轮转）】
  model.config_state: healthy | degraded | corrupt | recovering（恢复中，§3.5）
  监控镜像+指标（§3.4）
  migration：无 revision 的旧模型行 → 建 generation=1 基线 revision + active=1
```

```cpp
// ModelEntry 内存态——不可变快照（§3.1 第 8 步）
struct AppliedPostprocessSnapshot {          // 锁外预构造，提交点 noexcept 交换
    PostSessionHandle   post;
    PostprocessDocument applied_doc;
    std::string         applied_doc_hash;    // "c1:<hex>"
    std::string         canonical_version;
    uint64_t            applied_generation;  // PERSISTENT/RECONCILE 置 target；EPHEMERAL 不动
    uint64_t            runtime_revision;    // 每次 swap ++
    // overlay 身份：
    ApplyMode           apply_mode;          // 本态来源模式
    bool                overlay;             // EPHEMERAL 覆盖在途（重启即 false）
    uint64_t            overlay_base_generation;  // 覆盖前持久世代
    std::string         overlay_base_hash;        // 覆盖前文档 hash
    std::string         last_applied_json;   // 记录性，非真相
    std::string         last_op_id;
    ApplyResult         last_op_result;
    // 幂等环：预分配定长槽 N=16 {op_id, request_fingerprint(完整), result}
};
using PostprocessState = std::shared_ptr<const AppliedPostprocessSnapshot>;

// 实例身份（二分，runtime 域）：ModelEntry 增
uint64_t infer_instance_id;   // 每次"创建 inference 会话"的注册（fresh）随机 nonce；
                              // alias 复用 owner 的；unregister 后同 id 重注册=新 nonce
uint64_t entry_instance_id;   // 每条 registry 条目一个 nonce：fresh/alias 各自新；
                              // co-owner 加持不改（条目延续）
```

**overlay 语义**：EPHEMERAL 成功时记 `overlay=true` + `overlay_base_{generation,hash}`=**最近持久快照**的 (applied_generation, applied_doc_hash)——**链式覆盖不重捕**；**PERSISTENT/RECONCILE/重新注册清除**；**AdoptGeneration 在 hash 匹配时清除并归一 apply_mode=RECONCILE**（§3.4 分支 4）；等值 overlay 由分支 5 清除。reconciler 分支依据：hash 不等且 `overlay=true` → 保留（合法覆盖，除非平台 op 待收敛）；hash 不等且 `overlay=false` → 损坏/过期 → 安装。

**安装与管理标志**：平台管理标志=**DB 行存在性**；migration 后 DB 行 active≥1 恒成立，`initial_generation=0` 仅 transient/SDK 直注册。

**真相与健康**：DB active revision doc 为配置真相；健康两维 `content_converged`（hash 等）/`generation_aligned`（世代等）+ `overlay` 位 + `config_state`（含隔离/恢复中）。

### 1.4 Canonical 编码与 hash（完整文档 c1 / patch p1 双域）

```
CanonicalPostprocessDocument v1（完整文档域，前缀 "c1"）
  输入 = 规范化文档：别名折叠、decoder 默认填充、params 按 key 字节序排序、
         legacy_residual 经安全管线解析为 JSON 值后规范化（见下）
  参与字段：model_type、decoder、params 全量（含 Emit=false advisory）、legacy_residual
  排除字段：legacy_original、legacy_source（纯审计）、last_applied_json、op 元数据
  数值规则：
    - schema type=int 的键：double 须过整数性校验（isfinite 且 ==floor）→ int64 编码
    - 拒绝 NaN 与 ±Inf；-0.0 规范化为 +0.0
    - double = IEEE754 8B 大端位模式；int64 = 8B 大端二补码；bool = 1B
  **residual 递归 JSON 值编码**：
    值 = 类型标签 1B + 载荷：
      null | bool | int64（整数值时优先）| double | string（u32 长度前缀）|
      list（u32 计数 + 递归元素，保序）| object（u32 键数 + 键按字节序 + 递归值）
    深度 ≤8（安全管线解析界限同源）；对象键唯一（SAX 查重已保证）
  结构规则：字符串/字节段 = u32 大端长度前缀 + 内容；matrix = 行列表递归
  digest = SHA-256(canonical bytes)（版本前缀 "c1" 居首）；外部表示 "c1:<hex>"
  schema 升级 → 前缀升版（c2…）不互比；迁移=全量重算落库
  golden vectors：共享夹具（含 residual 嵌套对象/混合数组/null 变体），Go/C++ 同源，CI 双跑

CanonicalPostprocessPatch v1（patch 域，前缀 "p1"——①）
  用途 = REST/T2 的 delta 指纹与存储（U0'）——**不复用完整文档 canonicalizer**
  （完整文档侧填默认值会把三态折叠，执行效果不同的 delta 可能同指纹→误判幂等）
  presence 规则（核心）：
    - **在档键** = 显式编码（key + 值编码，含**空列表**——u32 计数 0 是合法编码）
    - **omitted** = 键不出现，**不产生任何字节**
    - **三态不可合并**：omitted ≠ set(空列表/显式清空) ≠ set(默认值)——
      执行效果不同，指纹必须不同；不设独立 clear 标签（clear 语义=
      set(空列表)（clearable 键）或 set(默认值)，均为显式编码）
    - 顶层身份字段（model_type/decoder）：patch 中出现即 set；值变化在
      U1 合成后/apply 第 4 步拒（registration identity）
  共享面（与完整文档同一实现，④ 不变量）：别名折叠、数值规范
  （NaN/±Inf 拒、-0.0→+0.0、整数性校验）、StringList/FloatList/FloatMatrix/
  residual 递归值编码；键按字节序排序
  digest = SHA-256(canonical bytes)（版本前缀 "p1" 居首）；外部表示 "p1:<hex>"
  golden vectors：omitted × empty-list × explicit-default 三态组合（各键型）、
  键序/空白无关性、别名折叠等价——Go/C++ 双跑
  fingerprint 域分离：runtime 环=r1（c1 输入）；平台 oplog=r2（p1 输入）——见 §1.1
```

### 1.5 注册协议 wire 增量

见 §1.1：`ModelRegisterRequest` 挂 `postprocess_doc(20)` / `initial_generation(21)` / `initial_doc_hash(22)`；**`ModelRegisterResponse` 挂 `infer_instance_id(3)` / `entry_instance_id(4)`**（现役仅 1-2，已核验）。token 面平台注册路径可设 `initial_generation=DB active`（migration 后 ≥1）+hash 核对；未鉴权者服务端覆写 0；`postprocess_doc` 缺省走旧字段翻译兼容。**fresh 注册同时生成 infer_instance_id + entry_instance_id；alias 仅生成 entry_instance_id（复用 owner 的 infer 号）；co-owner 两者皆不变**。

## 2. 注册：三条路径状态机

时序依据（已核验）：`backend_function` 在 `infer_ops_->create()` 前进 `platform_config`（model_manager.cpp:330-353）；HAL post create 从自身 config_json 读选择键（:1171/:1264）并自剥 loader 键（:980）；register_model 三分支（:150-284）。

### 2.1 fresh 路径（S0-S5）

| 步骤 | 动作 | 失败处理 |
|---|---|---|
| S0 静态校验 | 文档×schema（类型/decoder/逐键/重复键/legacy 安全管线/canonical 数值规则） | 拒绝，零副作用 |
| **S1 编译计划——三层产物** | ① `inference_platform_overrides`（loader 面，进 inference platform_config）；② `postprocess_loader_json`（HAL post create 的 config_json：decoder 选择键+合成参数+**Emit=true advisory 键**+平台受信表注入 lib 路径）；③ `plugin_payload`（派生断言面）。另附 expected_contract | 策略缺失/矛盾=拒绝 |
| S1' 派生规则 | **`plugin_payload := postprocess_loader_json − kLoaderKeys`**；Emit=false 键只记录不发射 | 快照测试断言等式 |
| S2 创建 inference 会话 | HAL create（①已注入） | 失败=拒绝 |
| S3 实测张量契约 | 真实 model_info × expected_contract | 回滚 S2 |
| S4 创建后处理+验证 | loader_json→init_post_session→ ValidateTier 验证（§3.1 第 7 步同款，含 FixtureBuilder） | 失败=回滚 S2 |
| S5 交付 | 冒烟（expectPostResult）；gRPC 直注册以 S3+S4 等价保障；token 面平台路径安装 initial_generation=DB active（+hash 核对）；**生成 infer_instance_id + entry_instance_id 并入 ModelEntry/响应** | 冒烟失败=回滚 S2 |

### 2.2 same-file alias 路径（A0-A5）

| 步骤 | 动作 | 失败处理 |
|---|---|---|
| A0/A1 | =S0/S1 | 同 |
| **A2 第一层等价门** | **先比较 ① inference_platform_overrides 与 owner 的规范化等价**（backend_function 等 loader 面设置）——不等价 → **拒绝 alias**（消息指引：以新 model_id 走 fresh 注册；不为此创建第二 inference 会话——资源与所有权边界） | REJECTED_SAFE |
| A3 契约校验 | 共享 model_info 张量契约 × expected_contract | 拒绝 |
| A4 语义等价 | ② 层 canonical 等价 → 共享 pp 会话（handle ref_count++）；不等价 → 独立 pp 会话 | — |
| A5 交付 | 插入 alias 条目：**新 entry_instance_id、复用 owner 的 infer_instance_id** | — |

**回滚只摘 alias 条目及其独立句柄，绝不触碰共享 inference 会话与 owner 条目。**

### 2.3 same-id co-owner 路径（C0-C2）

C0 规范化语义等价（§1.4 规则→深比较；取代 :186-190 裸串比较）→ 不等价=显式拒绝（含两侧摘要与 hash）→ C1 等价 rc=1 幂等 → C2 加 owner（**infer/entry 实例号皆不变**——条目与会话延续）。

## 3. 更新协议

### 3.1 执行原语：apply_document（幂等环 → mode 前置 → 快路径 → 实例 CAS + 不可变快照交换）

```cpp
// ModelManager::apply_document(model_id, doc, mode, base, target,
//                              expected(epoch,revision) 成对, op_id [, legacy_no_cas])
//   per-model 队列——与注册/注销共用同一生命周期串行化；mode 鉴权在 gRPC 入口
//   legacy_no_cas 仅由旧 UpdatePostprocessConfig 适配器置位（§3.2），新 RPC 不可达
0  实例捕获：entry_id = entry.entry_instance_id;
   infer_id = entry.infer_instance_id; infer = entry.infer_session（快照读）
1  幂等环查找（op_id 单键）：命中且完整指纹（mode/base/target/expected/canonical
   hash）相等 → 返回记录结果（历史重放，非新应用——先于一切校验）；
   命中且指纹异 → IDEMPOTENCY_KEY_REUSED（证明未应用）
2  模式前置检查（先于状态快路径）：
   PERSISTENT_CAS：base != current(applied_generation) → BASE_CONFLICT；
                    target < current → STALE；target == current → 继续（由 3/4 裁决）
   RECONCILE：target>current → 安装；==current 且 hash 异 → 安装（清 overlay）；
              ==current 且 hash 同 → 继续；<current → STALE（永不回退）
   EPHEMERAL：expected 成对且 (epoch,revision)!=(runtime_epoch,runtime_revision)
              → BASE_CONFLICT（跨重启 ABA 与延迟重放）；
              缺对且非 legacy_no_cas → 入口层拒绝（不可达）；legacy_no_cas → 无条件继续
3  状态相等快路径（位于 mode 校验后）：target==current 且
   canonical_hash(doc)==applied_doc_hash → IDEMPOTENT（不建会话）
4  校验：doc×schema；**身份字段（model_type/decoder）值变化 → REJECTED_SAFE
   （registration identity——decoder 决定 ① 层与 pp_session 存在性，须重注册）**；
   diff(applied_doc,doc)——create_only/requires_reload 仅"值变化"才拒；
   ValidateTier==reload_only → 拒——均 REJECTED_SAFE
5  translate → postprocess_loader_json（§2.1 三层之②）
6  new_pp = init_post_session(loader_json, entry.model_info)      // 锁外，不触碰旧会话
7  validate(new_pp, tier)（FixtureBuilder，见下）——失败 → destroy(new_pp)，REJECTED_SAFE
8  锁外预构造 AppliedPostprocessSnapshot{...}（overlay 身份：EPHEMERAL 记
   overlay=true 且 base 承载自现有快照的持久基线（链式覆盖不重捕，§1.3）；
   PERSISTENT/RECONCILE 置 overlay=false）
   lock: **CAS 校验 entry.entry_instance_id==entry_id 且 infer_instance_id==infer_id
         且 infer_session==infer**
         不匹配 → unlock，destroy(new_pp)，返回 BASE_CONFLICT（条目/会话已换代，候选安全废弃）
         匹配 → state_ptr = std::move(new_snapshot);   // 单次 noexcept 交换
   unlock——旧快照由在飞引用持有，post 句柄最后释放时 destroy
返回 APPLIED（响应附新 epoch/revision）
```

**验证分级（第 7 步）**：

```
constructive    → 参数回读/形状断言
synthetic_smoke → FixtureBuilder(model_info) 复刻 inference backend 的 ROI 附着张量
                  （HailoROIPtr 携带名称/dtype/量化/NMS 元数据——post run 对普通
                  HalTensor 直接 NOT_READY，:1439-1445）；零数据跑一次 post_process，
                  rc==0 即过；try/catch(...) 隔离插件异常；结果/fixture 按 HAL 所有权清理
无法构造等价 fixture → 保持 reload_only
```

**PostSessionHandle**：

```cpp
struct PostSessionDeleter { void operator()(HalPostprocessSession*) const; };
using PostSessionHandle = std::shared_ptr<HalPostprocessSession>;
```

- 现状锚点：`ModelSnapshot.post_session` 裸指针（model_manager.h:56-58）；`post_refs_`（:248）仅服务 unregister 别名——句柄统一取代。
- 语义：swap 后在飞请求继续用旧会话完成本帧；旧会话最后释放时销毁——无 UAF、无 grace timer；空数组清除=新会话无键。

**HAL 已知缺陷登记表**（swap+验证分级规避；单开 issue；**#2/#4 修复=P3 优先项，兼 CLIP 保级前置**）：

| # | 缺陷 | 锚点 | 规避 |
|---|---|---|---|
| 1 | apply 逐步改写 merged_vendor_json，中途失败留半应用态 | :439-486/:933-953 | swap 从不原地改 |
| 2 | CLIP scorer 重配失败仅告警仍返回成功 | clip 合并段 | 同上；**P3 优先修复** |
| 3 | 空 prompts/negative_prompts 不清旧值（num>0 门） | :885/:935 | 新会话无键即清除 |
| 4 | create 忽略 scorer configure 返回值/init 失败仅告警仍出会话 | create 路径 clip 段 | ValidateTier；**P3 优先修复** |

### 3.2 双通道入口

- **结构化通道（canonical）**：`ApplyPostprocessDocument`（§1.1）；平台控制面 PERSISTENT_CAS/RECONCILE（token），外部 gRPC 直连仅 EPHEMERAL（**成对 presence 强制，无缺省豁免**）。
- **legacy `config_json` 通道**：复用仓内 vendored `hal_v2/third_party/nlohmann/json.hpp`（ai-runtime 构建加 include path，无新增依赖）；SAX 解析（查重拒重复键；≤64KB、深≤8、数值界；malformed=fail-closed）→ family 反向别名映射 → canonical delta → **合并基线按入口区分**：
  - **REST 路径（PERSISTENT_CAS）**：platform-api 内以 **DB base revision doc** 为基线合并 delta（临时 overlay 永不合入持久真相；U2 的 base 校验同帧生效）；
  - **直连 gRPC 路径（旧 `UpdatePostprocessConfig` RPC）**：以 **runtime applied_doc** 为基线（覆盖语义本就如此），经**服务端适配器**转 `apply_document(..., legacy_no_cas=true, op_id=服务端 UUID v4)`——**唯一**的无 CAS 入口（显式内部路径、逐次审计 journal+指标、不暴露给新协议客户端）。

  汇入 apply_document。**并发语义明示**：legacy 通道无 CAS——last-writer-wins，过渡行为；**sunset=用量连续一个发布周期（或指定天数）为零后删除**（含 RPC 适配器与 legacy_no_cas 分支；观测面=oplog legacy_no_cas 列，§1.3）。
- 冲突规则：双通道同时非空=拒绝；同键经不同别名重复=拒绝。

### 3.3 REST update 提交状态机（T2：U0 → U0' 前置查重/新操作门 → U1 固定 base 合成 → 全 CAS 分派）

| 步骤 | 动作 | 失败结果 |
|---|---|---|
| **U0（② 纯静态）** | mode 鉴权（token）+ 请求外形（UNSPECIFIED 拒/EPHEMERAL 成对 presence 强制/op_id 长度与字符集/base,target 形状）——**静态检查，不读模型状态** | MODE_DENIED 等，**零副作用、不产生 DB 写入** |
| **U0' 前置幂等查重+新操作门（①②，先于读取 base revision；本步只读直至门禁）** | ① 规范化校验**提交 patch**（p1 域，§1.4——**不触 base/active**）→ fingerprint（r2）→ `oplog(model_id,op_id)` 查找：**命中且指纹同 → 返回原操作态**（终态原样——含 `failed(DB_INVARIANT_VIOLATION)`；pending/compensating → 202+Location）；**命中且异 → 拒绝（409，零 DB 写入）**；② **未命中 → 新操作门：`config_state∈{corrupt,recovering}` → 拒写**（隔离生效于"创建新操作"，不挡已有 op 的重试回读）；通过 → 进 U1 | KEY_REUSED（含审计四元组：stored/received fingerprint、conflict_count、correlation_id）；corrupt/recovering 拒写 |
| U1 | 新操作：delta×schema；**基线=revision(model_id, base_generation)（请求固定 base）**合成完整 desired（合成产物入 c1 域）；基线行不存在或当前 active≠base → 不合成，BASE_CONFLICT（调用方重读后**显式**以新 base 重提=可观察新操作） | 拒绝，零副作用 |
| U1' | desired 规范哈希核对（入 revision 存储；请求指纹已在 U0' 定型） | — |
| U2 | **单事务**：INSERT revision G+1（由固定 base 合成的 desired）+ `UPDATE active WHERE active==base`（CAS）+ INSERT oplog(pending, fingerprint=r2) | CAS 失败=**BASE_CONFLICT（不自动重读重试——隐式 rebase 禁止，R5）**；DB 失败=干净拒绝 |
| U3 | gRPC apply(mode=PERSISTENT_CAS, base=G, target=G+1, op_id)——按 ApplyResult 分派（下表）。追赶场景（重启后 runtime 落后）由 reconciler 走 RECONCILE | transport UNKNOWN：op 保持 pending，绝不立即补偿——同 op_id 重试（**U0' 幂等返回**）/hash 比对；超时窗后 reconcile 终态清算（§3.4A） |
| U4 | APPLIED/IDEMPOTENT → 单事务 oplog→committed | 不匹配→pending→reconcile |

**U3 结果分派表**：

| ApplyResult | 处置 | 理由 |
|---|---|---|
| APPLIED / IDEMPOTENT | U4 commit | 已生效 |
| REJECTED_SAFE | **补偿事务**：先 `UPDATE active WHERE active_generation==A.target(G+1)`（CAS）——**成功**才同事务分配补偿 revision（次代 G+2，=旧 doc，confirmed=false）+ 原 op→compensating → 尽力 inline AdoptGeneration；确认后→rolled_back。**CAS 失败**（active>G+1，已有后继）→ op→**superseded（不得补偿）**——防覆盖并发 B 的新版本 | 唯一常规可补偿类；补偿本身并发安全 |
| BASE_CONFLICT | 不终结：重读 DB active——==target 保持 **pending**（reconcile 安装）；>target → superseded | DB 已前移；直接 failed=永久分裂 |
| STALE | superseded（已有后继） | 同上 |
| MODE_DENIED | U0 已预检；漏网（token 配置漂移）→ 按 REJECTED_SAFE 走带 CAS 补偿 | 证明未应用 |
| IDEMPOTENCY_KEY_REUSED（二分） | **来源=DB 侧**：U0' 已提前拒（不可达）；**来源=runtime EPHEMERAL 环冲突**（DB 已推进）→ 按 REJECTED_SAFE 走**带 active-CAS 补偿**，op→compensating | 双真相消灭 |
| UNKNOWN（transport） | op 保持 pending；同 op_id 重试 / hash 比对；超时窗后 reconcile 终态清算 | runtime 可能已完成 swap |

- generation 单调性：append-only（回滚=前移到补偿 revision；补偿分配在 active-CAS 成功后，唯一约束保证无并发撞代）。
- op 终态：`committed | rolled_back | failed | superseded`；非终态：`pending | compensating`；failed 带 reason（MODEL_DELETED / DB_INVARIANT_VIOLATION / RECOVERY_RESYNC / …）。
- **GetPostprocessOperation = 平台管理面（platform-api REST，DB oplog 为源）**：
  `{op_id, model_id, target_generation, status, reason, detail, request_fingerprint, updated_at}`
  ——`request_fingerprint` 原样返回 `r2:<hex>`（patch 域；UI 可只示前 12 位）；KEY_REUSED 冲突摘要（stored/received fingerprint 最近一次）留审计事件；不暴露完整文档；重启后仍可答；gRPC 面仅 RuntimeState。
- **HTTP 形态（⑥）**：`pending | compensating` → **`202 Accepted`** + `Location: /operations/{op_id}`；任一终态 → **`200 OK`**；KEY_REUSED → **`409 Conflict`**；`Retry-After` 仅在服务端能给出稳定轮询建议时返回。响应 body 全状态同构（含 request_fingerprint）——SDK 对 200/202 用同一解析结构，不为"单一解析路径"把在途操作伪装成 200。

### 3.4 reconcile：全量扫描 + 两子相位（op 终态清算 → 内容对齐五分支）

**扫描范围**：周期任务覆盖**全部"DB 存在"的平台管理模型**——已加载者做内容对齐；**未加载者（重启/卸载后恢复中/实例换代）与 selfheal 装载协同比照**（触发装载→装载完成后再对齐，desired 安装到新实例）。oplog 扫描仅为加速器（定位待收敛模型）与终态判定输入。节律=固定周期+抖动（实现期定参，建议 30s 级）；per-model in-flight apply 期间跳过。**`config_state∈{corrupt,recovering}` 的模型跳过 A/B 两相位**——corrupt 的唯一出口是 §3.5 recovery-resync 管理流：若 reconcile 照跑 B 分支 1 会以 DB active 直接"自愈"安装，等于绕过恢复审计使隔离落空。

**A. op 终态清算（对每个有待定 op 的模型；恒跑）**：

| op 状态 | DB/runtime 观察 | 终态动作 |
|---|---|---|
| pending | active > target | **superseded**（后继已提交） |
| pending | active == target 且 runtime hash == active doc hash | **committed**（UNKNOWN/响应丢失的收敛通道，含 platform-api 重启后） |
| pending | active == target 且 hash 异 | 交 B 分支 1 安装 |
| pending/compensating | **active < target** | **隔离化**：模型 `config_state=corrupt`（新操作门拒写、读不受限）+ op → `failed`（**reason=DB_INVARIANT_VIOLATION**）+ 保留 revisions/oplog、不自动删除/倒推 active + 指标告警；恢复=§3.5 recovery-resync |
| compensating | active == 补偿 revision 且 runtime hash == 补偿 hash | confirmed=true + **rolled_back** |
| compensating | active == 补偿 revision 且 hash 异 | 交 B 分支 1 安装 |
| 任一 | DB 模型行存在、runtime 未加载（实例换代/恢复中） | **不终结**——协同 selfheal 装载，desired 由 DB active 定义，安装到新实例；实例号不入 oplog、不参与终态判定 |
| 任一 | DB 模型行已删除/显式取消 desired load | **failed（reason=MODEL_DELETED）** |

**B. 内容对齐分派（五分支）**：

1. hash 不等且 `overlay=false` → RECONCILE(target=DB active，单向：>current 安装；==current 且 hash 异=清 overlay 安装；<current=并发倒挂，拒+重读)；
2. hash 不等且 `overlay=true` 且无平台待定 op → **保留 overlay**（合法 EPHEMERAL；健康 `content_converged=false, overlay=ephemeral`）；
3. hash 不等且 `overlay=true` 且有平台待定 op（pending/compensating）→ 按分支 1 安装（归一）；
4. hash 相等且 generation 不等 → `AdoptGeneration(expected_hash, G_active)`：仅前移元数据；**成功即 overlay=false、清 overlay_base、apply_mode 归一 RECONCILE**；调用前后核验 DB active 仍==G_active；成功→compensation confirmed=true、compensating→rolled_back；
5. hash 相等且 generation 相等 → 稳态确认（无动作）；若 `overlay=true`（等值覆盖）→ 清 overlay 位归一（幂等元数据重建，无 DB 面）。

**重启语义（显式协议条款）**：EPHEMERAL overlay **不跨进程生命周期**——runtime 重启后按 DB active（平台管理模型）装载：overlay=false、生成**新 epoch**；**携带旧 epoch 的 Apply 一律 BASE_CONFLICT**。临时覆盖的存续期=当前进程生命周期，重启即隐式归一到持久态（设计特性）。测试链条见 §7。

- **RECONCILE 双读**：调用 runtime **前后各重读** DB active——前读定 target；后读发现已推进 → 立即调度最新版本安装。
- **ephemeral overlay**：仅 `runtime_revision++`、更新 applied_doc/hash/overlay 身份，不动 applied_generation；归一仅 (a)PERSISTENT/RECONCILE (b)重注册 (c)AdoptGeneration/分支 5 (d)重启 四类时机；延迟重放/跨重启保护=(epoch,revision) 成对 CAS。
- **指标**：最老 pending 时长、UNKNOWN 数、补偿未确认数、generation lag、hash mismatch 计数、ephemeral overlay 存量、**legacy_no_cas 使用计数（由 oplog legacy_no_cas 列派生——sunset 判据跨重启可查）**、corrupt/recovering 计数、KEY_REUSED conflict_count。
- **退避**：复用 modelHealBackoff；持续失败→journal 限频+`config_state` 呈现。
- oplog 落盘：U0 拒绝（计数）、U0'/U2/U3 分派/reconcile A/B 各分支每次提交/补偿/采纳/放弃/**KEY_REUSED 审计事件**。

### 3.5 corrupt 隔离恢复：recovery-resync（REST admin 唯一入口，③）

**裁决依据**：`config_state`、revisions、active 指针、oplog 全部由 platform-api/DB 拥有——直连 ai-runtime gRPC 只能重装内存态，**无法原子修复或解除 DB 隔离**。故恢复面单开 **platform-api REST admin endpoint 为唯一外部入口**，不新增运维向 ai-runtime 管理 gRPC；普通 ApplyMode **不设穿透参数**（ai-runtime 侧仅使用既有 capability token 保护的 RECONCILE）。

```text
POST /admin/models/{model_id}/postprocess/recovery-resync
{
  "strategy": "reload_active" | "restore_revision",
  "source_generation": 12,          // restore_revision 必填
  "reason": "operator explanation"
}
```

| 步骤 | 动作 | 失败处理 |
|---|---|---|
| R1 鉴权+锁 | 独立 admin RBAC/token 鉴权（区别于 apply capability token）；取得模型配置锁（与 T2/reconcile 同一串行化域） | 401/403/409 |
| R2 中间态 | `config_state: corrupt → recovering`（recovering 期间 U0' 新操作门同样拒写；读面不受限） | — |
| R3 源校验/复制 | `reload_active`：校验当前 active revision 完整性（行存在、doc 可解析、hash 自洽）；`restore_revision`：**不倒退 active 指针**——把指定旧 revision **复制成新的最高 generation**（append-only 保持；active 以 CAS 前移到该新代）——恢复本身可审计、可再次回退 | 校验失败 → 保持 recovering，返回详情 |
| R4 审计 | oplog 记 `reason=RECOVERY_RESYNC` + 操作者 + 来源 revision（source_generation） | — |
| R5 安装核对 | platform-api 以**既有内部 RECONCILE** 安装到 runtime，核对 runtime hash/generation 与 DB 一致 | 失败 → 保持 corrupt/recovering（可再次 resync） |
| R6 终裁 | **完全核对成功 → `config_state=healthy`**；任何失败保持 corrupt/recovering | — |

**启动清扫**：platform-api 启动时全表扫描——`recovering` 一律回退为 `corrupt`（R6 的完全核对未完成即不得停留于中间态；若 R5 核对已过而 R6 未提交，回退后重跑 reload_active 幂等无害）+ 指标告警提示运维重新 resync。中间态因此**不跨 platform-api 进程生命周期**（与 EPHEMERAL overlay 的重启条款同构）。

## 4. 方言翻译层

### 4.1 输出合成：三层产物（每 decoder 恰一个 Synthetic）

| Synthetic | decoder | 产物 |
|---|---|---|
| `hailortpp_3key` | hailo_yolov8n/s/m（参数化） | ② loader_json=backend_function + 受信 lib 路径 + 3 消费键 + Emit=true advisory 键；③=②−kLoaderKeys；① overrides=backend_function |
| `fixed_name` | yolov5m_vehicles、yolov5、yolov8s/m、yolox… | ②=backend_function+受信 lib 路径；③=空；遗留键 advisory+Emit=false；① overrides=backend_function |
| `pose_blob` | native_yolov8_pose | ②=`native_yolov8_pose:true`+参数包+create 期网络尺寸；③=参数包；①=空 |
| `passthrough` | mediapipe/linknet/clipgen/SCDepth/OCR | 零/自有键直通（②=③） |
| `raw` | — | 无 post 会话（§5） |
| `generic_yolo_7key` | （Q5 开放） | 七键全发（②含全部；③=②−kLoaderKeys） |

三层各自快照测试（①进 inference platform_config；②进 HAL post create；③按 S1' 等式断言）。

### 4.2 legacy→canonical 输入翻译 + 控制键映射表（T3）

| legacy 输入形态 | 翻译结果 |
|---|---|
| 裸名 ∈ 参数化白名单 | decoder=该名，params={} |
| 裸名 ∈ 固定名集合 | decoder=该名；七键 blob 其余键→advisory+Emit=false 参数 |
| detection 七键 blob | decoder=backend_function；3 键→consumed；3 advisory→advisory |
| keypoint 完整 blob | decoder=native_yolov8_pose；识别键→canonical；未识别键→安全管线 residual |
| DB 旧行/旧 AMPK | 按形态分派；不可安全解析者只留 original（residual 恒空，不执行） |

**legacy 安全管线（residual 唯一产出路径）**：

```
nlohmann SAX 解析（查重拒重复键；≤64KB/深≤8/数值界）
  → 必须是 JSON object → 控制键提取/黑名单（下表）→ 键分类
  → 通过：residual = 剩余已分类键值（合成可消费；按 §1.4 递归编码参与 hash）
  → 失败：residual 恒空；仅 original 留档；synthesis 忽略——绝不把未检文本送回 HAL
```

**控制键映射与黑名单**：

| legacy 键 | 处理 |
|---|---|
| `backend_function` | 提取；必须等于 doc.decoder，不等=拒绝 |
| `backend_lib_path` / `backend_config_path` | 拒绝（09-20 禁令；lib 路径只由平台受信表注入） |
| `native_yolov8_pose`（bool，:1162） | 提取；与 decoder 一致性校验；永不 opaque |
| `yolov8_pose_network_width/height` | create_only 参数，非切换键 |
| 未识别非控制键 | 安全管线分类后进 residual；detection 封闭类型拒绝携带 |

冲突规则：params 与 legacy_residual 同键=拒绝。

### 4.3 双向别名表

按 family 隔离；已知行：`detection_threshold`≡`confidence_threshold`（detection family；keypoint 同名键独立）、`max_boxes`↔`max_detections`。P2 录入冻结。

## 5. raw decoder 契约（P4；实测基线 09-20）

1. StreamInfer outputs 附带条件 `!enable_post` → `!enable_post || !pp_session`（修 untyped 无 flag 空 payload，grpc_service.cpp:2429）；infer() 单发不动；行为变化入发版说明。
2. proto TensorSpec 增：`quant{scale,zero_point,range_min,range_max}`、`nms{number_of_classes,max_bboxes_per_class,max_bboxes_total}`、`semantic_dtype`。
3. SDK 配套：`parse_hailo_nms(buffer, nms_info)`（1.12.1 布局）；BoundingBox 左上角式文档；金标准脚本进 `tests/device/`。

## 6. 兼容矩阵的实现落点

| 既有面 | 实现 |
|---|---|
| platform.db 旧行 | 读时翻译不改写；migration 建 gen=1 基线+active 指针；回滚=旧代码直接可用 |
| SDK 旧签名（model_type+model_variant） | 收时翻译；proto 只增 optional（Request 20+/Response 3+，现役上界已核验） |
| 旧 update config_json（含数组键） | §3.2 legacy 通道（SAX+别名）；**REST 以 DB base 合并、直连 gRPC 经旧 RPC 适配器 legacy_no_cas（服务端 UUID op_id+审计）**；last-writer-wins 无并发保证（明示） |
| 既有热更能力（CLIP prompts、OCR det_* 等） | 未过保级前置前=breaking change：拒热更+reload 工作流+发版说明；P3 优先修 HAL readiness；清单裁决不静默 |
| gRPC 无 type 注册 | 非 transient→decoder:"raw"；transient→维持 raw_output_only gate |
| 存量 keypoint 开放 blob | 安全管线 residual（控制键先剥离；§1.4 递归编码参与 hash） |
| detection 七键 blob | §4.2 翻译行；advisory 3 键+UI 徽标 |
| 厂商插件方言 / HAL ABI | 冻结零改动（CLIP readiness 修复为 P3 优先独立小改） |
| **存量直连 gRPC 无 CAS 更新** | **仅经旧 `UpdatePostprocessConfig` RPC 适配器（legacy_no_cas 显式内部路径）**；逐次审计/指标（oplog 列持久观测）；**sunset=用量连续一个发布周期（或指定天数）为零后删除**——新结构化 RPC 无任何缺省豁免 |

## 7. 测试计划

| 层 | 内容 |
|---|---|
| Go 单测 | schema round-trip；翻译快照；过渡合成器快照；**U0-U4 + U3 全结果分派**；**U0' 前置查重+新操作门**（同 op_id 在 active 已从 base 推进后重试 → 命中返回原操作，**不进 U1**；**corrupt 模型上触发隔离的 op 重试 → 返回 failed(DB_INVARIANT_VIOLATION) 而非通用拒写**；corrupt/recovering 模型新 op_id → 拒写；异指纹拒+审计四元组）；**CanonicalPostprocessPatch**（omitted × empty-list × explicit-default 三态组合→不同 digest；键序/空白无关；别名折叠等价；与完整文档共享规范化实现的回归）；**U2 固定 base 合成**（base 行缺失/active≠base → BASE_CONFLICT）；**无隐式 rebase**；补偿 active-CAS；KEY_REUSED 二分；compensating→确认→rolled_back；CAS 并发；migration 首次 base=1；AdoptGeneration（前移/hash 不符/DB 漂移拒/清 overlay+归一 RECONCILE）；RECONCILE 双读；**reconcile 全量扫描**（committed 模型破坏 hash→修复；oplog 空也能修；未加载模型→协同装载→安装到新实例；DB 删除→failed(MODEL_DELETED)）；**A 表终态清算**（swap 成功→响应丢失→platform-api 重启→pending 收敛 committed；active>target→superseded）；**corrupt 隔离流程**（active<target → corrupt+failed(DB_INVARIANT_VIOLATION)+新操作门拒写+保数据；§3.5 恢复）；**recovery-resync**（reload_active 成功/校验失败保持 recovering；restore_revision 复制为新代**不倒退 active**；R5 失败保持 recovering；完全核对成功→healthy；审计字段 operator/source_generation）；GetPostprocessOperation（终态机/reason/request_fingerprint 外显）；**HTTP 形态**（202+Location/pending、200/终态、409/KEY_REUSED；Retry-After 条件返回）；B 表五分支；op_id 校验；指标；oplog 审计 |
| C++ stub 单测 | S0 负例；三层快照+S1' 等式；**A2 第一层等价门**；swap 并发/生命周期（TSAN/UAF）；**实例 CAS**；快照提交点原子性；validate 分级+FixtureBuilder；generation 语义（UNSPECIFIED 拒/幂等环 op_id 单键+全指纹（r1 域）/快路径位于 mode 校验后/新 RPC 缺 expected 对=入口拒（legacy_no_cas 不可达）/跨重启 nonce 拒）；**身份字段值变化拒**；三路注册（实例号二分断言）；canonical golden vectors 双语言（**c1 完整文档+residual 变体、p1 patch 三态**）；"只拒变化的 create_only"；SAX 查重/界限；**旧 RPC 适配器路径**（legacy_no_cas 置位+审计 journal+服务端 UUID op_id） |
| CI | 双侧 schema 重生成 diff；golden vectors 双跑（c1/p1 双域） |
| **随机并发状态机测试** | ① U2→并发更新→U3 失败→补偿（补偿永不覆盖后继；**同 op_id 重试在 U0' 返回原操作**）；② 进程重启后旧 EPHEMERAL 请求（epoch nonce 拒；**legacy_no_cas 仅旧 RPC 可达**）；③ Apply×重新注册并发（实例 CAS）——不变量：DB active 链单调、runtime 态∈{DB 任一 revision doc ∪ 合法 overlay 基线}、无悬空非终态、desired 永不基于非请求 base 合成、**op 终态判定不依赖实例号**、**同 op_id 同 r2 指纹的重试永不产生第二个 DB 写** |
| 设备 | P1 姿态零 JSON 导入；P3a 只读面；P3b：双会话 A/B、ValidateTier 探测、重启恢复、**响应丢失收敛**、补偿→AdoptGeneration 闭环、**重启清 overlay 链条（overlay → crash/restart → DB active 装载 → overlay=false → 旧 epoch Apply → BASE_CONFLICT）**、UNKNOWN 注入、**corrupt 隔离与 recovery-resync 人工恢复（REST admin 全流程）**；kill 注入；P4 金标准 bit-exact+量化元数据 |
| 回归 | 09-20 整改全量用例保持绿 |

## 8. 阶段交付物映射

| 阶段 | 触及文件（新增★/修改） | 独立验收 |
|---|---|---|
| **P1 keypoint 档案** | ★`modelload/legacysynth`；`model_types.go`；前端；`bundled_package.go`（metadata v2） | 姿态零手写 JSON 导入出 COCO-17；facial 表单零"填了无效"键 |
| **P2 schema 单源** | ★schema 源/generator/生成头、CI、`model_variant_validation.cpp` 改查表；矩阵+别名+ValidateTier/Fixture 定级冻结 | 单点改双侧生效；单侧手改 CI 拦 |
| **P3a 只读面** | ★proto（§1.1 全套，含 Response 3/4）；★canonical 编码器（**c1 完整文档+residual 递归编码**）+golden vectors；migration；`grpc_service.cpp` RuntimeState；三层翻译/语义等价接线 | golden vectors 双语言一致（含 residual 变体）；migration 后 base 合法；读回=canonical 文档 |
| **P3b 持久 apply** | `grpc_service.cpp`（apply/token 鉴权/双通道/op_id 校验/旧 RPC legacy_no_cas 适配器）；`model_manager.*`（PostSessionHandle/双实例号/apply_document 步序/ValidateTier+FixtureBuilder/AdoptGeneration/不可变快照）；nlohmann include；Go：revisions+oplog（唯一约束+reason/legacy_no_cas/r2 指纹）+T2（U0'/新操作门/固定 base 合成）+★CanonicalPostprocessPatch 编码器+★configReconciler（全量扫描+两子相位+双读+装载协同+corrupt 隔离）+★GetPostprocessOperation(REST，含 fingerprint 外显+**202/200/409 形态**) +★**recovery-resync admin endpoint（独立 RBAC）**+指标（含 legacy_no_cas 计数）；**capability token（0600/轮换/双令牌）**；**HAL CLIP readiness 修复（P3 优先）**；删除 legacysynth | U0-U4 全分派+随机并发状态机+kill 注入收敛+补偿 active-CAS+实例 CAS 绿；U0' 重试（含 corrupt 回读）/响应丢失收敛/重启清 overlay/corrupt 隔离+recovery-resync 全流程验证；**patch p1 三态指纹**；validate 设备定级；旧面全兼容（breaking 项发版说明）；CLIP 保级复评 |
| **P4 SDK 收敛** | SDK：`register_model(doc=)`、`apply_postprocess_document`(EPHEMERAL，成对强制)/`get_postprocess_runtime_state`、`parse_hailo_nms`、deprecated 双轨、**retry(existingOperation)/apply(newOperation) 区分+KEY_REUSED 标准文案**；**legacy_no_cas 适配器按 sunset 条件删除** | 双轨全绿；raw 全链含元数据；金标准 E2E 进套件；legacy_no_cas 用量归零一个发布周期后删除 |

依赖：P1 可即启；P2 前置=矩阵全表；P3a 前置=P2+评审通过；P3b 前置=P3a；P4 前置=P3b。HAL 基线零改动；CLIP readiness 修复为 P3b 独立小改（优先）。

## 9. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 双 pp 会话瞬时并存 | 按模型串行+句柄自动释放；P3 定级，不容忍者 reload_only（R6）；未探测默认 reload_only |
| ValidateTier/FixtureBuilder 误判 | fixture 复刻真实 ROI 契约；设备 A/B 抽测；降级 reload_only（可降不可升需复测） |
| 既有热更能力回归 | breaking 明示+reload 工作流；P3 优先修 HAL readiness；清单裁决不静默 |
| canonical 编码跨语言漂移 | §1.4 规范（**c1/p1 双域**：含 residual 递归编码与 patch 三态 presence）+版本前缀+golden vectors 双语言 CI |
| **patch 三态误折叠**（omitted/清空/默认值同指纹→误判幂等） | p1 独立编码不填默认；三态组合 golden vectors；指纹比对仅在同域（r2）内 |
| 补偿/并发边界误操作 | 全部指针推进=带期望代 CAS；补偿先 CAS 后分配；仅 REJECTED_SAFE（及漏网 MODE_DENIED/EPHEMERAL 环冲突）补偿；随机并发状态机+kill 注入 |
| **隐式 rebase 覆盖并发修改** | U2 固定 base 合成；CAS 失败=BASE_CONFLICT 不自动重试；rebase 只能显式新操作（R5） |
| 实例换代替换污染 | 双实例号提交 CAS（entry+infer+会话指针）；注册/注销/apply 同一串行化；候选废弃路径测试；**换代不终态化 op（装载协同）** |
| 世代倒挂/回写旧 revision | RECONCILE 单向+调用前后双读；AdoptGeneration 仅前移+前后核验；**active<target → corrupt 隔离不倒推（不掩盖根因、无悬空非终态）** |
| overlay 误判（损坏当合法覆盖） | overlay 身份显式存于快照；PERSISTENT/RECONCILE/重注册/AdoptGeneration/分支 5/重启 清除；查询 RPC 暴露 |
| **稳态漂移不可修复** | reconcile 全量扫描（含未加载模型装载协同）；oplog 仅加速器 |
| 鉴权面缺口 | token 主路径（0600/轮换/双令牌）；SO_PEERCRED/local credentials 列后续增强（现状 InsecureServerCredentials 已核验） |
| **恢复面滥用/误操作** | recovery-resync 独立 admin RBAC+审计（operator/source_generation 入 oplog）；recovering 中间态防半恢复；restore_revision 复制为新代不倒退 active；**不设 ApplyMode 穿透参数** |
| **legacy 无 CAS 面被新客户端利用** | **豁免隔离在旧 RPC 适配器（显式 legacy_no_cas 内部路径）**，新结构化 RPC 强制成对 presence；逐次审计+指标（oplog 列持久观测）；用量归零一个发布周期后删除（可删边界） |
| generation/overlay 协议复杂度 | 状态机表+全失败路径+随机并发+kill 注入；hash 判等简化心智；SDK 只暴露简单 API |
| 语义等价误判 | 规范化表驱动+正负例快照；不等价消息含两侧摘要与 hash；alias 第一层等价门正负例 |
| proto 字段号碰撞 | 预留区 Request 20+/Response 3+（现役上界已核验）；合入前终裁+扫 Go 调用点+重生成 ARM proto |
| 提交点部分失败 | 不可变快照锁外预构造+锁内单次 noexcept 交换；幂等环定长槽；构造异常注入测试 |
| nlohmann 复用面扩大 | 仅 legacy 边缘 SAX；canonical 纯 proto；界限防 DoS |
| Go 过渡合成器漂移 | P1 仅 2 策略+快照；P3b 强制退役 |
| 矩阵与 vendor 漂移 | 基线 1.12.1；升级触发重核；advisory 判定保守 |
| legacy 后门 | original 永不参与合成与 hash；residual 只出自安全管线；控制键强制提取+校验；封闭类型拒携 |

## 10. 与提案的互引

- R1-R4 对应提案评审 #1/#2/#3/#6/#7；R5/R6 经九轮评审演化（append-only+全 CAS+固定 base+前置幂等+hash 收敛+typed 失败分类+补偿 CAS+单向恢复+终态清算 / 会话级原子+双实例 CAS+不可变快照+句柄生命周期）。
- §3.1 HAL 缺陷登记表（4 项）为提案范围外 issue 线；#2/#4 修复=P3 优先项（兼 CLIP 保级前置）。
- 提案 §8 Q5/Q6 不阻塞。
- 本 v10 关键机制：三路注册状态机+第一层等价门+双实例身份（§2）、apply_document 步序+T2 U0' 前置查重/新操作门+固定 base 合成+reconcile 全量扫描两子相位/装载协同/corrupt 隔离/重启条款+recovery-resync（§3）、三层翻译+legacy 安全管线+legacy_no_cas 隔离（§3.2/§4）、**c1/p1 双域 canonical+三态 presence+r1/r2 双域指纹**（§1.1/§1.4）、两步上线（P3a/P3b）。

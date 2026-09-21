# 统一后处理配置设计方案（Unified Postprocess Config — Design）

> Status: **design draft v11 — 范围收口"砍到核心"（2026-09-20），未批准实施**
> **原始目标**：导入模型时能正常使用设备内置后处理与自定义后处理。
> v10→v11：九轮评审演化的强一致机制**整体降为冻结附录**（全文存档于 [unified-postprocess-config-strong-consistency.md](./unified-postprocess-config-strong-consistency.md)，触发条件见 §11）。正文只保留服务原始目标的核心；**放松的保证（诚实清单）**：
> - 并发写：`WHERE active==base` 单句 CAS——后到者收到 BASE_CONFLICT 后**显式**重读重提（无自动 rebase）；
> - 安装失败不回滚 DB：真相已推进、运行态滞后 → **读时对账/重启自愈**（不再有补偿事务与后台 reconcile 循环）；
> - SDK 重试安全=**内容幂等**（hash 相等即 no-op），不再有 op_id/指纹；
> - legacy 直连 gRPC tweak=运行态本地修改，**不承诺跨读回/跨重启存活**（diverged 明示+显式同步/采纳，重启归一）。
> 依据：提案 v2；官方 postprocess 库 1.12.1；09-20 裸输出链 93.72 实测；锚点核验累计（post run 要求 ROI 附着 TensorPriv hailo15_postprocess_impl.cpp:1439-1445 / backend_function 于 infer create 前注入 platform_config model_manager.cpp:330-353 / native_yolov8_pose create 期开关 :1162 / ModelRegisterRequest 现役至 17）。

---

## 0. 设计总览

```
Web 向导 │ REST │ SDK(gRPC) │ AMPK 包          ← 四入口，全部提交统一文档
    ▼
┌─ platform-api / app-manager（Go）──────────────────────┐
│ 组装文档；schema 校验；DB append-only revisions +        │
│ active 指针（单事务 CAS，基线=请求固定 base）；migration │
│ （旧行 gen=1 基线）；读时对账（§3.4）；指标            │
└────────────────────────────────────────────────────────┘
    ▼  gRPC InstallPostprocessDocument(doc, expected_generation)   [内部面]
┌─ ai-runtime（C++）─────────────────────────────────────┐
│ 注册：三路径状态机（fresh/alias/co-owner，§2）          │
│ 安装：per-model 串行 → 内容幂等 → 世代守卫 → 校验       │
│       → 三层翻译 → 新建会话 → 验证 → 锁内快照 swap     │
│ 旧 UpdatePostprocessConfig：原样保留=运行态本地 tweak   │
└────────────────────────────────────────────────────────┘
    ▼
HAL（零改动基线；CLIP readiness 修复为 P3 优先项，§3.1 缺陷表）
```

六条设计红线（R5 较 v10 简化）：

- R1 接受≠消费：每键 `effect` 标注，`advisory` 键 UI 必带不生效提示；
- R2 热更逐键 `mutability`；**`model_type`/`decoder` 为注册身份字段，值变化一律拒绝**；`create_only/requires_reload` 显式拒绝（仅拒**值变化**的键）；
- R3 张量契约校验在真实 model_info 存在后执行（注册期两阶段），错配注册时拒绝；
- R4 raw 量化元数据是 P4 前置；
- R5' **DB=平台管理模型配置真相；单事务 CAS（无隐式 rebase）；安装失败不回滚真相，读时对账/重启自愈**；
- R6 **会话级原子 + 生命周期安全**：swap 不依赖 HAL 原子性；提交点=锁内单次 noexcept 快照交换；旧会话由最后在飞快照释放销毁；**无任何 destroy-then-create 回退**。

## 1. 核心数据结构

### 1.1 统一配置文档与安装协议（proto，可编译示意）

```protobuf
// inference.proto 增量（proto3，已核验）。新 message 字段号自 1 起；
// ModelRegisterRequest 增项挂 20+（现至 17=batch_size）。
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

// 平台内部安装 RPC（platform-api → ai-runtime；非公开 SDK 面；
// 鉴权=网络位置信任，同现有内部 RPC 现状——无 token 体系，见 §6）
enum InstallResult {
  INSTALL_RESULT_UNSPECIFIED = 0;  // 客户端/入口拒绝
  APPLIED   = 1;
  IDEMPOTENT = 2;   // 内容幂等：hash 与世代均等（重试安全网）
  REJECTED  = 3;    // 校验/构造/验证失败——未应用
  STALE     = 4;    // expected_generation 落后于运行态（乱序丢弃）
}
message InstallPostprocessDocumentRequest {
  string              model_id = 1;
  PostprocessDocument doc      = 2;
  uint64 expected_generation   = 3;  // =平台 DB active（守卫乱序/迟到安装）
}
message InstallPostprocessDocumentResponse {
  InstallResult result           = 1;
  string        message          = 2;
  uint64        applied_generation = 3;
  uint64        runtime_revision = 4;
  string        applied_doc_hash = 5;  // "c1:<hex>"
  string        canonical_version = 6;
}

// 运行态查询（读回对账数据源）
message GetPostprocessRuntimeStateRequest { string model_id = 1; }
message GetPostprocessRuntimeStateResponse {
  string      applied_doc_hash   = 1;
  string      canonical_version  = 2;
  uint64      applied_generation = 3;   // 最近一次平台安装的世代（legacy tweak 不动）
  uint64      runtime_revision   = 4;   // 每次 swap ++
  string      last_source        = 5;   // platform | legacy_local（diverged 判定输入）
}

// 注册增量：
// ModelRegisterRequest 挂：
//   PostprocessDocument postprocess_doc = 20;  // 缺省=走旧 model_type/model_variant 翻译
//   uint64 initial_generation            = 21;  // >0 仅平台注册路径可用；其余服务端覆写 0
//   string initial_doc_hash              = 22;  // 平台核对（canonical hash）
```

- 重复键语义：`params` 同键两次=拒绝（§3.2 SAX）。
- `decoder:"raw"`：params/legacy 恒空。
- **REST 是唯一持久写入口**（platform-api 自身认证即鉴权）；`InstallPostprocessDocument` 为平台内部面，SDK 不暴露。外部 SDK 直连的配置修改仅剩旧 `UpdatePostprocessConfig`（§3.2）。

### 1.2 schema 单源（typed 约束 + 验证分级）

`platform/platform-api/model/postprocess_schema.go`（Constraint/ParamDef/TensorExpect/DecoderSpec 同 v3，含 ValidateTier）：

```go
type ValidateTier string // constructive | synthetic_smoke | reload_only

type DecoderSpec struct {
    // ...（v3 字段不变：ModelTypes/Decoder/Source/FixedName/Params/Hardcoded/
    //      Contract[NamePattern,Dims,CountMin/Max/Mod,Dtype,IsNMS]/Synthetic）
    Validate ValidateTier   // 判据=能否在"新建第二会话+验证"交换协议内确认可用
    Fixture  string         // synthetic_smoke 的 FixtureBuilder 标识（§3.1 第 6 步）
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
platform.db（持久，Go 拥有；append-only + 指针）
  postprocess_revisions: model_id, generation(单调), doc, created_at
                         **UNIQUE(model_id, generation)**——并发插入在 DB 层互斥
  model 行 active_generation 指针——**推进=单事务 CAS：UPDATE ... WHERE active_generation=<请求 base>**
  migration：无 revision 的旧模型行 → 建 generation=1 基线 revision + active=1
  监控指标：安装失败数（REJECTED/transport）、读时对账触发数、diverged 模型数、
           legacy 直连通道使用计数（sunset 观测面——metrics/journal，无 oplog）
```

```cpp
// ModelEntry 内存态——不可变快照（§3.1 第 7 步）
struct AppliedPostprocessSnapshot {          // 锁外预构造，提交点 noexcept 交换
    PostSessionHandle   post;
    PostprocessDocument applied_doc;
    std::string         applied_doc_hash;    // "c1:<hex>"
    std::string         canonical_version;
    uint64_t            applied_generation;  // 平台安装世代（legacy tweak 不动）
    uint64_t            runtime_revision;    // 每次 swap ++
    std::string         last_source;         // platform | legacy_local
    std::string         last_applied_json;   // 记录性，非真相
};
using PostprocessState = std::shared_ptr<const AppliedPostprocessSnapshot>;
```

**真相与对账**：DB active revision doc 为配置真相；运行态健康由**读时对账**维持（§3.4）：世代落后→自动补装；世代相等 hash 异→`diverged`（明示，不自动覆盖）。**无后台 reconcile 循环、无 config_state 机、无 oplog**（强一致版见附录）。

**安装与管理标志**：平台管理标志=**DB 行存在性**；migration 后 DB 行 active≥1 恒成立，`initial_generation=0` 仅 transient/SDK 直注册。

### 1.4 Canonical 编码与 hash（完整文档 c1 单域）

```
CanonicalPostprocessDocument v1（前缀 "c1"）
  输入 = 规范化文档：别名折叠、decoder 默认填充、params 按 key 字节序排序、
         legacy_residual 经安全管线解析为 JSON 值后规范化（见下）
  参与字段：model_type、decoder、params 全量（含 Emit=false advisory）、legacy_residual
  排除字段：legacy_original、legacy_source（纯审计）、last_applied_json
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
```

c1 的三个用途：①安装/读回的**内容幂等判等**（重试安全网）；②co-owner 语义等价比较（§2.3）；③revision 行身份（防存储层静默改写）。（patch 域指纹 p1/r1-r2 随附录冻结。）

### 1.5 注册协议 wire 增量

见 §1.1：`ModelRegisterRequest` 挂 `postprocess_doc(20)` / `initial_generation(21)` / `initial_doc_hash(22)`。token 面平台注册路径可设 `initial_generation=DB active`（migration 后 ≥1）+hash 核对；未鉴权者服务端覆写 0；`postprocess_doc` 缺省走旧字段翻译兼容。

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
| S4 创建后处理+验证 | loader_json→init_post_session→ ValidateTier 验证（§3.1 第 6 步同款，含 FixtureBuilder） | 失败=回滚 S2 |
| S5 交付 | 冒烟（expectPostResult）；gRPC 直注册以 S3+S4 等价保障；token 面平台路径安装 initial_generation=DB active（+hash 核对） | 冒烟失败=回滚 S2 |

### 2.2 same-file alias 路径（A0-A5）

| 步骤 | 动作 | 失败处理 |
|---|---|---|
| A0/A1 | =S0/S1 | 同 |
| **A2 第一层等价门** | **先比较 ① inference_platform_overrides 与 owner 的规范化等价**（backend_function 等 loader 面设置）——不等价 → **拒绝 alias**（消息指引：以新 model_id 走 fresh 注册；不为此创建第二 inference 会话——资源与所有权边界） | REJECTED |
| A3 契约校验 | 共享 model_info 张量契约 × expected_contract | 拒绝 |
| A4 语义等价 | ② 层 canonical 等价 → 共享 pp 会话（handle ref_count++）；不等价 → 独立 pp 会话 | — |
| A5 交付 | 插入 alias 条目 | — |

**回滚只摘 alias 条目及其独立句柄，绝不触碰共享 inference 会话与 owner 条目。**

### 2.3 same-id co-owner 路径（C0-C2）

C0 规范化语义等价（§1.4 规则→深比较；取代 :186-190 裸串比较）→ 不等价=显式拒绝（含两侧摘要与 hash）→ C1 等价 rc=1 幂等 → C2 加 owner。

**注销竞态不变量**（替代 v10 双实例号）：安装/更新与注册/注销共用**同一 per-model 串行队列**——队列内不存在注销与安装并发，无需提交点实例 CAS。（若实现期发现队列外路径，再评估局部守卫，不引全局机制。）

## 3. 更新协议

### 3.1 执行原语：install_document（串行 → 内容幂等 → 世代守卫 → 校验 → 会话交换）

```cpp
// ModelManager::install_document(model_id, doc, expected_generation)
//   per-model 队列——与注册/注销/legacy update 共用同一串行化（§2.3 不变量）
1  内容幂等快路径：canonical_hash(doc)==applied_doc_hash 且
   expected_generation==applied_generation → IDEMPOTENT（不建会话）
2  世代守卫：expected_generation < applied_generation → STALE（乱序/迟到安装丢弃）；
   其余继续
3  校验：doc×schema；**身份字段（model_type/decoder）值变化 → REJECTED
   （registration identity——decoder 决定 ① 层与 pp_session 存在性，须重注册）**；
   diff(applied_doc,doc)——create_only/requires_reload 仅"值变化"才拒；
   ValidateTier==reload_only → 拒——均 REJECTED
4  translate → postprocess_loader_json（§2.1 三层之②）
5  new_pp = init_post_session(loader_json, entry.model_info)      // 锁外，不触碰旧会话
6  validate(new_pp, tier)（FixtureBuilder，见下）——失败 → destroy(new_pp)，REJECTED
7  锁外预构造 AppliedPostprocessSnapshot{...}（last_source=platform）
   lock: state_ptr = std::move(new_snapshot);   // 单次 noexcept 交换
   unlock——旧快照由在飞引用持有，post 句柄最后释放时 destroy
返回 APPLIED（响应附 applied_generation/runtime_revision/hash）
```

**验证分级（第 6 步）**：

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

- **结构化通道（canonical）**：`InstallPostprocessDocument`（§1.1）——仅 platform-api 调用（REST 是唯一持久写入口，platform-api 自身认证即鉴权；内部 gRPC 面鉴权=网络位置信任，同现状）。
- **legacy `config_json` 通道**：复用仓内 vendored `hal_v2/third_party/nlohmann/json.hpp`（ai-runtime 构建加 include path，无新增依赖）；SAX 解析（查重拒重复键；≤64KB、深≤8、数值界；malformed=fail-closed）→ family 反向别名映射 → canonical delta → **合并基线按入口区分**：
  - **REST 路径**：platform-api 内以 **DB base revision doc** 为基线合并 delta，随后走 §3.3 更新流（与结构化通道同一条 CAS 路径）；
  - **直连 gRPC 路径（旧 `UpdatePostprocessConfig` RPC）**：以 **runtime applied_doc** 为基线，经现有 HAL apply 路径直接生效（fail-loud 整改后行为）。**文档明示语义：运行态本地修改（last_source=legacy_local）**——读回呈现 diverged、不跨重启存活（重启按 DB active 装载）、后续平台安装/读时对账会归一；sunset 观测=metrics/journal 使用计数，用量归零一个发布周期后评估删除。

- 冲突规则：双通道同时非空=拒绝；同键经不同别名重复=拒绝。

### 3.3 REST 更新流（U1 校验合成 → U2 单事务 CAS → U3 安装 → U4 返回）

| 步骤 | 动作 | 失败结果 |
|---|---|---|
| U1 | delta×schema；**基线=revision(model_id, base_generation)（请求固定 base）**合成完整 desired；基线行不存在或当前 active≠base → BASE_CONFLICT（调用方重读后**显式**以新 base 重提） | 拒绝，零副作用 |
| U2 | **单事务**：INSERT revision G+1 + `UPDATE active WHERE active==base`（CAS） | CAS 失败=BASE_CONFLICT（不自动重读重试）；DB 失败=干净拒绝 |
| U3 | 内部 gRPC Install(doc, expected_generation=G+1)——REJECTED/transport 失败：**DB 保留新真相**，响应="已保存，运行时安装失败：<原因>"（+指标；读时对账/重启自愈，§3.4）；STALE=并发后继已在装（读回确认）；APPLIED/IDEMPOTENT→U4 | 见左 |
| U4 | 返回 {generation=G+1, applied_doc_hash} | — |

注：绝大多数校验/翻译失败在 **U1/U2 前置**拦下（DB 不动）；U3 的 REJECTED 仅剩运行期构造/tier 验证类失败（罕见）——此时真相领先、运行态滞后，属 §3.4 自愈窗口。**无补偿事务、无 oplog、无后台循环**。

### 3.4 读时对账 + 重启语义

```
GET 配置读回（platform-api）时比对 runtime（GetPostprocessRuntimeState）：
  A. runtime.applied_generation < DB active（或 runtime 无态）
     → 后台触发一次 Install(current active doc)——修 U3 失败/崩溃窗口；指标计数
  B. 世代相等且 hash 异（大概率 legacy 直连 tweak，last_source=legacy_local）
     → **diverged=true，不自动覆盖**；响应附两个显式动作：
       〔同步配置到运行态〕= Install(DB active doc)
       〔采纳运行态为配置〕= 以运行态 doc 走一次正常 §3.3 更新（新 revision）
  C. 世代相等且 hash 同 → 稳态
重启：平台管理模型一律按 DB active 装载（legacy tweak 归一——与今天"重启无持久化"行为一致）
```

- 读回主面=**DB 文档**（真相）；运行态单独呈现（generation/hash/last_source/diverged）——两者不混排。
- 指标：对账触发数（A）、diverged 存量（B）、安装失败数、legacy 通道使用计数。

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
| SDK 旧签名（model_type+model_variant） | 收时翻译；proto 只增 optional（Request 20+，现役上界已核验） |
| 旧 update config_json（含数组键） | §3.2 legacy 通道（SAX+别名）；REST 路径=DB base 合并走 §3.3 CAS；直连 gRPC=运行态本地 tweak（diverged 明示、不跨重启、读时对账归一） |
| 既有热更能力（CLIP prompts、OCR det_* 等） | 未过保级前置前=breaking change：拒热更+reload 工作流+发版说明；P3 优先修 HAL readiness；清单裁决不静默 |
| gRPC 无 type 注册 | 非 transient→decoder:"raw"；transient→维持 raw_output_only gate |
| 存量 keypoint 开放 blob | 安全管线 residual（控制键先剥离；§1.4 递归编码参与 hash） |
| detection 七键 blob | §4.2 翻译行；advisory 3 键+UI 徽标 |
| 厂商插件方言 / HAL ABI | 冻结零改动（CLIP readiness 修复为 P3 优先独立小改） |
| 内部 gRPC 鉴权现状 | 网络位置信任（同现有内部 RPC；无 token 体系——强一致版的 capability token 随附录冻结，见 §11 触发条件） |

## 7. 前端呈现层

前端零自有键表——三处共用同一 schema 源（P2 Go 导出）与同一 `ModelSchemaField` 渲染器（现骨架：向导 Upload→Parse→Configure，`web/src/pages/ai-models/`）。

| 面 | 内容 | 阶段 |
|---|---|---|
| **A 导入向导 Configure 步** | ①"后处理档案"选择器（多 decoder 类型显示；单解码器隐式；raw=输出张量预览卡+输入契约提示，无参数表单）；②每键元数据徽标：effect（生效/〔不生效〕/仅记录——R1）、mutability（🔒 create_only/↻ hot/⟳ requires_reload）；③硬编码档案（facial 468 点等）显示说明文字不出表单项；④JSON 逃生口提交后返回安全管线反馈（识别键提升/进 residual 折叠/坏格式红条"仅留档不执行"） | P1 起步（keypoint 双档+raw 预览） |
| **B 模型详情·后处理面板** | 状态行（generation/c1 hash 前 12 位/last_source/diverged 徽标）；生效参数读回（=DB 文档，advisory 带徽标）；运行态子区（runtime_revision）；"原始导入 JSON"审计折叠（legacy_original 只在此，永不与生效参数混排）；diverged 时横幅+双按钮（同步配置到运行态/采纳运行态为配置） | P3 |
| **C 更新抽屉** | 只渲染 hot_update 键（requires_reload 置灰+重载工作流链接）；BASE_CONFLICT 黄条=〔重读最新〕〔基于新 base 重新提交〕（均显式新提交，无自动 rebase）；提交响应直接呈现 U3 结果（含"已保存、安装失败：将于读回/重启时重试安装"形态） | P3 |

（op 进度页/修订历史页/恢复页随强一致机制冻结于附录。）

## 8. 测试计划

| 层 | 内容 |
|---|---|
| Go 单测 | schema round-trip；翻译快照；过渡合成器快照；U1-U4（base 行缺失/active≠base→BASE_CONFLICT；CAS 并发后到者拒；无隐式 rebase）；migration 首次 base=1；REST legacy 合并基线=DB base；**读时对账三分支**（落后→补装；diverged→不覆盖+双动作；稳态）；采纳运行态=新 revision；canonical c1 golden vectors；指标 |
| C++ stub 单测 | S0 负例；三层快照+S1' 等式；A2 第一层等价门；swap 并发/生命周期（TSAN/UAF）；install 步序（内容幂等快路径先于一切/STALE 乱序丢/身份字段值变化拒/reload_only 拒/"只拒变化的 create_only"）；validate 分级+FixtureBuilder；注册三路径（alias 共享/独立、co-owner 语义等价正负例）；SAX 查重/界限；legacy 直连路径 last_source=legacy_local；golden vectors 双语言 |
| CI | 双侧 schema 重生成 diff；golden vectors 双跑 |
| 随机并发状态机 | REST 更新×并发 REST 更新（后到者 BASE_CONFLICT）；REST 更新×legacy 直连 tweak（diverged→显式动作归一）；安装×注销（同队列串行不变量）——**不变量**：active 链单调、runtime 态∈{DB 任一 revision doc ∪ divergent legacy tweak}、无隐式 rebase、内容幂等重试无第二副作用 |
| 设备 | P1 姿态零 JSON 导入出 COCO-17；P3：双会话 A/B、ValidateTier 探测、重启恢复（=DB active 装载+legacy tweak 归一）、**U3 失败注入→读时对账自愈**、legacy tweak→diverged→采纳/同步双路、kill 注入；P4 金标准 bit-exact+量化元数据 |
| 回归 | 09-20 整改全量用例保持绿 |

## 9. 阶段交付物映射

| 阶段 | 触及文件（新增★/修改） | 独立验收 |
|---|---|---|
| **P1 keypoint 档案** | ★`modelload/legacysynth`；`model_types.go`；前端（A 面 decoder 下拉+硬编码说明）；`bundled_package.go`（metadata v2） | 姿态零手写 JSON 导入出 COCO-17；facial 表单零"填了无效"键 |
| **P2 schema 单源** | ★schema 源/generator/生成头、CI、`model_variant_validation.cpp` 改查表；矩阵+别名+ValidateTier/Fixture 定级冻结 | 单点改双侧生效；单侧手改 CI 拦 |
| **P3 统一文档+读回+简版更新** | ★proto（§1.1）；★canonical c1 编码器+golden vectors；migration；`grpc_service.cpp`（Install RPC+RuntimeState+注册接线/三层翻译）；`model_manager.*`（PostSessionHandle/install_document/ValidateTier+FixtureBuilder/不可变快照）；nlohmann include；Go：revisions+CAS 更新流+读时对账+指标；前端 B/C 面；**HAL CLIP readiness 修复（P3 优先）**；删除 legacysynth | golden vectors 双语言一致；migration 后 base 合法；读回=canonical 文档（写读往返一致）；U1-U4+随机并发+kill 注入收敛绿；U3 失败注入自愈验证；validate 设备定级；旧面全兼容（breaking 项发版说明）；CLIP 保级复评 |
| **P4 SDK 收敛** | SDK：`register_model(doc=)`、`get_postprocess_config`/`get_postprocess_runtime_state`、`parse_hailo_nms`、deprecated 双轨、旧 UpdatePostprocessConfig 标记 deprecated（用量归零一个发布周期后评估删除） | 双轨全绿；raw 全链含元数据；金标准 E2E 进套件 |

依赖：P1 可即启；P2 前置=矩阵全表；P3 前置=P2+评审通过；P4 前置=P3。HAL 基线零改动；CLIP readiness 修复为 P3 独立小改（优先）。

> **实施状态（2026-09-20）：P1 + P2 已同批落地**（核心方案，本版 v11）。与上表规划的具体差异：
> - **P2 单源落点** = 新叶子包 `platform/postprocess`（`registry.go` 注册表 + `generate.go` go:generate 入口，不 import 仓内其它包）；生成物 checked-in：`platform/postprocess/generated/postprocess_schema.json` 与 `platform/ai-runtime/include/postprocess_schema.h`（`kModelTypes`/`kDetectionBackends`/`kDetectionVariantKeys`/`kKeypointVariantKeys`/`kForbiddenVariantKeys`/`kDecoders[]`）。漂移检查三面挂载：`make postprocess-schema-check`、`scripts/run_basic_tests.sh`、ci.yml 显式步骤。`model_variant_validation.cpp` 已改查生成表，错误文案由数组拼接。
> - **P1 合成器挂点** = `modelload.RuntimeRegistration` 内（未建独立 `modelload/legacysynth` 包）；keypoint 档案键复用 `postprocess_profile`，值域 `facial_landmarks`（缺键默认）/ `yolov8_pose`；`num_keypoints` 已从表单 schema 删除；keypoint blob 校验 = 结构检查 + loader 键黑名单（`backend_lib_path`/`backend_config_path`），不做闭集（延续 legacy opaque 通道裁决）。`bundled_package.go` 经 synth 补 InputWidth/Height，无 metadata v2 新格式。
> - **labels 的 effect 标注**：设备 A/B 已复核（2026-09-20，93.72，hailo_yolov8n_384_640）——改 labels 配置后 live post_result 标签随变（person/face → zzz_foo/yyy_bar），**参数化入口（hailo_yolov8n/s/m）labels 为 consumed**；表单侧 metadata 徽标已撤（注册表 per-decoder 真值本就正确，固定名入口如 yolov5m_vehicles 仍烘焙标签表、忽略该键）。

## 10. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 双 pp 会话瞬时并存 | 按模型串行+句柄自动释放；P3 定级，不容忍者 reload_only（R6）；未探测默认 reload_only |
| ValidateTier/FixtureBuilder 误判 | fixture 复刻真实 ROI 契约；设备 A/B 抽测；降级 reload_only（可降不可升需复测） |
| 既有热更能力回归 | breaking 明示+reload 工作流；P3 优先修 HAL readiness；清单裁决不静默 |
| canonical 编码跨语言漂移 | §1.4 规范（含 residual 递归编码）+版本前缀+golden vectors 双语言 CI |
| **U3 失败后的自愈窗口**（DB 领先、运行态滞后） | 读回主面=DB 真相（不呈现过期运行态为生效值）；读时对账 A 分支补装+指标告警；窗口有界（下次读/重启） |
| **diverged 误判/滞留** | 仅同代 hash 异才 diverged（世代判据先于 hash）；UI 双显式动作+横幅不放任；重启天然归一 |
| 并发更新互踩 | 单事务 CAS（base 固定）；BASE_CONFLICT 显式重读重提；无隐式 rebase |
| 注销/安装竞态 | 同一 per-model 串行队列（§2.3 不变量）；随机并发测试覆盖 |
| 语义等价误判 | 规范化表驱动+正负例快照；不等价消息含两侧摘要与 hash；alias 第一层等价门正负例 |
| proto 字段号碰撞 | 预留区 Request 20+（现役上界已核验）；合入前终裁+扫 Go 调用点+重生成 ARM proto |
| 提交点部分失败 | 不可变快照锁外预构造+锁内单次 noexcept 交换；构造异常注入测试 |
| nlohmann 复用面扩大 | 仅 legacy 边缘 SAX；canonical 纯 proto；界限防 DoS |
| Go 过渡合成器漂移 | P1 仅 2 策略+快照；P3 强制退役 |
| 矩阵与 vendor 漂移 | 基线 1.12.1；升级触发重核；advisory 判定保守 |
| legacy 后门 | original 永不参与合成与 hash；residual 只出自安全管线；控制键强制提取+校验；封闭类型拒携 |
| **放松保证被未来需求突破** | §11 触发条件清单；突破时解冻附录（v10 已含完整状态机与评审结论，演进不是从零开始） |

## 11. 与提案的互引 + 附录触发条件

- R1-R4 对应提案评审 #1/#2/#3/#6/#7；R5'/R6 为核心版红线（强一致完整版见附录）。
- §3.1 HAL 缺陷登记表（4 项）为提案范围外 issue 线；#2/#4 修复=P3 优先项（兼 CLIP 保级前置）。
- 提案 §8 Q5/Q6 不阻塞。
- **附录（[unified-postprocess-config-strong-consistency.md](./unified-postprocess-config-strong-consistency.md)，冻结 v10）触发条件**——出现任一即解冻评审后启用：
  1. 多写者并发配置编排（编排器/多客户端同时驱动配置变更，last-writer-wins 不可接受）；
  2. SDK 需要 at-least-once 强重试语义（内容幂等不足以满足的场合：需区分"同一操作的重试"与"新操作"）；
  3. 无人值守场景要求崩溃中安装的自动终态清算与隔离恢复（corrupt 隔离+recovery-resync）；
  4. 配置操作需要外部可查询的操作账本（oplog/GetPostprocessOperation 审计面）。

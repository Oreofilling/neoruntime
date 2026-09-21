# 统一后处理配置提案（Unified Postprocess Config）

> Status: **draft v2 — 提案，未批准实施**（v2 2026-09-20：按评审 7 条意见全量修订；§3.1 重构为经官方源码核实的解码器能力矩阵）
> Origin: 2026-09-20 模型导入后处理审计与整改（fail-loud 系列 7 笔提交）的后继设计讨论 + 同日提案评审
> Scope: **平台层**配置模型统一。不改动厂商插件方言（.so 已编译冻结）、不改动 HAL `HalPostprocessConfig` ABI。
> 证据基线: 官方 postprocess 库 [hailo-media-library **1.12.1**](https://github.com/hailo-ai/hailo-media-library/tree/1.12.1/hailo-postprocess)——vendor 库升级是 §3.1 能力矩阵的重核触发器。

---

## 0. TL;DR

后处理配置今天散在四层、同一字段藏着三种语义、两个白名单表靠注释人肉同步，而且**"HAL 接受的键"与"解码器实际消费的键"从未分开清点**——官方源码核实显示多组平台接受的键在插件里是硬编码（填了不报错、也永不生效）。本提案以**按 decoder 的能力矩阵**（实际消费键 / 硬编码 / 张量契约 / mutability）为地基，把平台层统一为一份配置文档 `{model_type, decoder, params}`：schema 单源、逐键标注生效阶段与消费方、注册期校验 decoder 与模型输出的张量契约、update 按键声明可变性（不支持热改的显式 `requires_reload` 拒绝）。分四阶段落地，P1（keypoint decoder 档案）收益独立可验。

## 1. 背景与问题

### 1.1 四层碎片化

| 层 | 现状 | 统一程度 |
|---|---|---|
| 网页表单 | `model_types.go` 一张 schema 表驱动所有类型的动态表单 | ✅ 机制统一，键按类型 |
| 平台键空间 | REST / gRPC / AMPK / update 四入口共用一套键 | ✅ 键空间统一（单表） |
| 注册 variant | detection 七键选择器 / keypoint pose 参数包 / 其余类型不消费 | ❌ 三种语义 |
| HAL/插件 | C 结构体按类型分子段 + 各插件自有 JSON 方言 | ❌ 无法统一（冻结） |

### 1.2 具体痛点（均有代码/设备/官方源码佐证）

1. **variant 字段三种语义**（`model_manager.cpp` `init_post_process` 仅处理 detection 与 keypoint 两个分支）：detection 空=默认档案/裸名=backend_function/JSON=封闭七键；keypoint 空=人脸默认/裸名=拒绝/JSON=pose 参数包（`native_yolov8_pose` 必须注册时给）；其余类型字段不参与。
2. **命名空间分裂**：同一个"检出阈值"在表单、DB 列、variant blob、update 键里各叫各的，靠"列提升"胶水互译。
3. **双白名单人肉同步**：Go `DetectionPostprocessProfiles` 与 C++ 校验表（`model_variant_validation.cpp`）靠注释互相提醒，无机器保障。
4. **keypoint pose 无 UI 入口**：姿态模型只能手写完整 JSON blob 激活。
5. **接受 ≠ 消费**（v2 新增，评审 #1）：HAL `kNumericKeys` 接受 `confidence_threshold`/`keypoint_threshold`/`num_keypoints`，但 mediapipe 人脸插件**不读任何配置**（468 点、0.5 阈值、192×192 归一化全部硬编码）；平台 detection 七键 blob 中 `iou_threshold`/`output_activation`/`label_offset` 对白名单 hailortpp 函数**不生效**（其配置 schema 仅 3 键）。这是"校验成功但运行无效"配置项的根源。
6. **方言漂移**：别名族（`detection_threshold`≡`confidence_threshold` 等）、同名异义（`score_threshold` 在 CLIP 与 pose 含义不同）、结构差异（个别方言有数组/布尔键）。

## 2. 目标 / 非目标

**目标**

- G1 用户与 SDK 面对一套配置语义：`{model_type, decoder, params}`，与类型无关的同构操作。
- G2 参数 schema **单源**：一张表定义每类型的合法 decoder 与参数键——名称/类型/范围/默认之外，逐键标注 **mutability**（create_only / hot_update / requires_reload）、**effect**（consumed / advisory / metadata）、**dialect_key** 映射、可否清空、读回来源；每 decoder 附**张量契约**。Go 与 C++ 从同一源生成或加载，漂移在 CI 拦截。
- G3 注册与 `update_postprocess_config` 编辑**同一文档**的同一命名空间；update 按键声明的 mutability 执行——不支持热改的键显式返回 `requires_reload`，**绝不接受后静默无效**。
- G4 decoder 与模型输出张量的**契约在注册期校验**（对照 model_info 的张量名/数量/形状/dtype），错配在注册时拒绝而非首帧才失败。
- G5 厂商方言、HAL C 结构体、既有 DB 行、AMPK 包、SDK 签名全部**兼容过渡**，无一次性断裂。
- G6 保持既有安全属性：detection 封闭 schema、backend 白名单、loader 键全入口拒绝。

**非目标**

- 不改厂商插件 .so 的 JSON 方言（冻结）；不动 `HalPostprocessConfig` 布局与 HAL ops 表。
- 不追求"所有类型都有 decoder 选择器"——单解码器类型只有隐式默认。
- 不在本提案内重写 Web 前端框架；表单仍由 schema 驱动动态渲染。

## 3. 现状盘点（已核实事实，作为设计输入）

### 3.1 解码器能力矩阵（v2 重构：官方 1.12.1 源码逐文件核实）

先立三层概念——**接受层**（HAL/平台解析，填了不报错，`kNumericKeys` 两张表 `hailo15_postprocess_impl.cpp:406/:2197` + 字符串键 `:447`）、**消费层**（decoder 实际读取并改变行为）、**硬编码层**（编译期常量，配置永远改不动）：

| decoder | 实现来源 | 消费层：实际读取的键 | 硬编码层 | 接受层收下但不生效的键 |
|---|---|---|---|---|
| `hailo_yolov8n/s/m`（yolo_hailortpp 参数化入口） | 官方 libyolo_hailortpp | `labels` `detection_threshold` `max_boxes`（schema 恰 3 键，全消费） | 输出张量名（`hailo_yolov8n_384_640/yolov8_nms_postprocess` 等）、COCO 标签表、`filter_by_score=true` | `iou_threshold` `output_activation` `label_offset`（平台七键 blob 的其余键） |
| yolo_hailortpp **固定名入口**（`yolov5` `yolov8s/m` `yolox` `*_vehicles`…） | 官方 libyolo_hailortpp | **无**（不接收 params） | 同上 + 各自标签表 | 全部配置键（2026-09-02 设备实测 yolov5m_vehicles 运行期阈值 NO-OP 与此吻合） |
| generic YOLO 函数（`yolo_postprocess.cpp`） | 官方 libyolo | **七键全消费且全 required**：`iou_threshold` `detection_threshold` `output_activation` `label_offset` `max_boxes` `anchors[][]` `labels`（labels 数与类数运行时强校验） | 无 | 未入平台白名单 |
| `facial_landmarks_nv12` | 官方 libmediapipe | **无任何配置** | 468 点、presence 0.5、192×192 归一化、张量名 `face_landmarks_lite/conv22`+`conv25` | `confidence_threshold` `keypoint_threshold` `num_keypoints` |
| linknet 分割 | 官方 liblinknet | **无任何配置** | mask 0.5、单通道、每输出张量一类 | `confidence_threshold` `output_width` `output_height` |
| CLIP embedding 3 编码器 | 官方 libclipgen | **无任何配置** | embedding 维度（640/512/768）、单张量、1D 假设 | `score_threshold` `top_k` |
| CLIP 分类（clip.cpp） | 官方 libclip | `prompts` `embedding` `negatives` `threshold`——经 **ZeroMQ 运行时消息**，非配置文件 | logit_scale=100、`"A photo of "` 前缀、`index+3`、无 top_k | `top_k` `confidence_threshold` |
| `native_yolov8_pose` | HAL 内置 | `confidence/score/keypoint/iou_threshold` + `yolov8_pose_network_width/height`——**仅 create 时**从 config_json 读（`hal_internal_yolov8_pose.cpp:507`） | COCO-17 | 网络尺寸不在两张热更新键表中（update 静默跳过） |
| OCR 检测/识别 | 内置 Paddle/libocr | `det_*` / charset 族键 | — | — |
| SCDepth | HAL 内置 | `scdepth_output_name` `depth_float32` | — | — |

三点推论：

- **平台 detection 七键 schema 与官方 generic YOLO 七键几乎同构**（`anchors`↔`backend_function` 之差）——历史渊源在此；但白名单 4 函数在 yolo_hailortpp，schema 仅 3 键。白名单各函数逐一归属参数化/固定入口是 **P2 的核查项**（上表已标出已知者）。
- "能填的键"里有相当比例属于接受层幻影；**任何 schema 若不区分 effect，就会批量生成"校验成功但运行无效"的配置项**（评审 #1 的核心警告）。
- clip 分类的 prompts 走 ZeroMQ 通道，而 `model_manager.h:207` 注释宣称 update 支持 "CLIP zero-shot prompts"——通路待核（§8）。

### 3.2 HAL 已有"方言吸附层"萌芽

- `sync_hal_postprocess_scalar`（别名键归一进 C 结构体，按类型分派）、`kNumericKeys`（跨类型数值键总表）。二者属于**接受层**——这正是 §3.1 区分三层的理由。翻译层事实上已存在于 HAL 内部，本提案把它提升到平台层并配正式 schema 与消费方标注。

### 3.3 既有校验底座（2026-09-20 整改产物，直接复用）

- gRPC `RegisterModel` 校验 model_type（13 类型）+ variant（detection 七键闭合 + 4 项 backend 白名单 + keypoint 裸名拒绝）；
- `update_postprocess_config` 错误码可读化；装载冒烟对 post 模型强制 `post_result`；HAL 对不可用插件 fail-loud + 注册回滚。
- 边界：REST 装载路径的冒烟会跑真推理、能拦"错 decoder → rc≠0"；但 **gRPC/SDK 直注册路径无冒烟**，且"能跑但解错"的语义错配冒烟也拦不住——这是 §4.3 张量契约校验的动因（评审 #6）。

## 4. 设计

### 4.1 统一配置文档（平台层唯一表示）

每模型一份文档，三段结构：

```json
{
  "model_type": "keypoint",
  "decoder": "native_yolov8_pose",
  "params": {
    "confidence_threshold": 0.6,
    "keypoint_threshold": 0.25,
    "network_width": 640,
    "network_height": 640
  }
}
```

- `model_type`：语义不变（13 类型，别名归一沿用）。
- `decoder`：**新增显式选择层**。各类型在 decoder 注册表里列出合法值（含各自张量契约）；单解码器类型允许省略（=唯一默认）。
- `decoder: "raw"`：**裸输出是一等选项**——平台不解码，只回原始输出张量，后处理由应用自行实现（`params` 为空，配置页只剩输出张量预览与输入契约提示）。与今天"不带 model_type 注册"的区别是**显式声明**：注册期即可区分"有意裸输出"与"漏配类型"（09-20 SDK test_06 的 -2 事故正是后者被当成前者）。**前置验收项**（评审 #7）：输出张量的量化元数据（scale/zero-point）必须随模型信息暴露——TensorSpec 现只有 name/shape/dtype/layout/byte_size，量化字段未落地前，raw 模式的承诺收窄为"无需反量化即可消费的模型"。
  **回传契约（93.72 实测 09-20）**：`infer()` 单发路径**无条件**附输出张量；`StreamInfer` 仅在请求带 `raw_output_only=true` 时附——untyped 模型**不带该 flag 订阅会得到 success=true + 空载荷**（`grpc_service.cpp:2429` `enable_post = has_post_ops() && !raw_output_only`，而 untyped 无 `pp_session`，两个分支都不进——又一例"校验成功但运行无效"）。另：运行期输出 HalTensor 一律按字节面送回（dtype=uint8、shape=字节数），NMS 输出流语义为 float32，SDK 侧须自行 `view(np.float32)`；NMS 内存布局为官方 1.12.1 `hailo_nms_decode.hpp` 契约（按类分段 `f32 count` + 每框 5×f32 `(ymin,xmin,ymax,xmax,score)`、类号=段序+1），但**类数/max_boxes 等 NMS 元数据未随 model_info 暴露**，应用解析只能行进至缓冲尾——随量化元数据一并列入 P4 暴露面。
- `params`：键由 schema 按 `(model_type, decoder)` 定义，**平台命名空间**，翻译层映射到方言键。

### 4.2 schema 单源

- **源**：Go 定义 + 导出（倾向，编译期类型检查），定义每 decoder 的参数键与张量契约。
- **每键字段**（v2 扩充，评审 #2）：`name/type/range/default/枚举` + **`mutability`**（`create_only` | `hot_update` | `requires_reload`）+ **`effect`**（`consumed` 该 decoder 真读 | `advisory` 接受但不生效，UI 须带警告徽标 | `metadata` 仅记录）+ **`dialect_key`**（译到方言的键名，含别名族）+ **`clearable`**（数组键可否清空）+ **`read_source`**（读回来源：注册文档 / 运行时态 / 不可读）。
- **每 decoder 字段**：实现来源、函数 ABI、**张量契约**（期望输出张量名/数量/dtype/布局）、消费键集与硬编码集（即 §3.1 矩阵的机器可读化）。
- **消费**：Go 侧生成 `ModelFieldDef`（表单机制零改动）；C++ 侧**构建期生成** `postprocess_schema.h` 静态数组（ai-runtime 不引 JSON 库），`model_variant_validation.cpp` 改查生成表；CI 校验双侧一致。
- 安全不变量写入源：loader 键永不进 `params`；decoder 白名单即 schema 一部分；`advisory` 键在 UI 强制带"不生效"提示。

> **落地注记（2026-09-20，P2 随 P1 同批实施）**：单源已实现为 Go 叶子包 `platform/postprocess`（`registry.go`，不 import 仓内其它包）；`go generate ./platform/postprocess` 产出 checked-in 生成物 `platform/postprocess/generated/postprocess_schema.json` 与 `platform/ai-runtime/include/postprocess_schema.h`，后者即本节设想的静态数组形态（含 `kModelTypes` 13 类型、decoder 白名单、detection 7 键闭集、keypoint 8 键消费集、loader 键黑名单、全矩阵 `kDecoders[]`，每键带 mutability/effect/dialect_key）。`model_variant_validation.cpp` 已改查生成表；漂移由 `make postprocess-schema-check` 在本地基础测试脚本与 CI 两面拦截。`ModelFieldDef` 经 effect/profiles 字段直通前端（表单机制零改动）。

### 4.3 三入口一出口（含张量契约校验）

```
Web 向导 / REST / SDK / AMPK 包
        │  全部提交统一文档
        ▼
  统一文档校验三件套：
    ① 类型/decoder 合法（schema 表）
    ② params 逐键（schema：类型/范围/mutability/effect）
    ③ 张量契约（decoder 注册表的期望输出 × model_info 实际张量：名/数量/形状/dtype）
        │
        ▼
  方言翻译层（DetectionVariantJSON 的泛化；advisory 键原样保留进方言 blob）
        ▼
  RegisterModel（gRPC，带 type+variant）→ HAL → 插件/内置解码
```

- 契约数据源：HEF vstream 信息——Web 侧 `suggestPostprocessProfile` 已有 germ，HAL `kFamilyFunctions` 警告启发式可退役并入契约表；
- REST 装载冒烟继续作为端到端兜底；张量契约校验补上 **gRPC 直注册路径无冒烟**的空档，并把"错家族但能跑"的语义错配从首帧前移到注册时；
- Web 向导"后处理档案"下拉泛化为 decoder 选择器；SDK 增可选 `decoder`/`params`，旧 `model_type`+`model_variant` 字符串翻译兼容；AMPK metadata 增补 `decoder`。

### 4.4 注册与 update 同文档（v2 收窄承诺，评审 #3）

- `update_postprocess_config(model_id, params_delta)`：delta 应用到统一文档 `params`，经同一 schema 校验后重译方言并走 HAL apply 路径——但**热更新能力逐键由 `mutability` 声明，不再整段承诺**：
  - `hot_update` 键（如各阈值类，在 `apply_config_json` 的 `kNumericKeys` 表内）可热改；
  - `create_only` 键（实证：pose 的 `yolov8_pose_network_width/height` 不在任何一张热更新键表，解码器 create 时读——update 提交它们会被静默跳过）**必须显式拒绝并返回 `requires_reload`**，消灭新的静默 no-op；
  - 阈值类键的生效路径还依赖 config_json 与结构体的读序，P2 建 matrix 时逐键核清 `read_source`。
- `decoder` 与 `model_type` 仍只能注册时定（HAL create 选后端，现有契约不变，文档明示）。
- **配套读回 API** `get_postprocess_config(model_id)`：返回统一文档（写→读往返一致是 P3 验收项）。**依赖声明**（评审 #4）：当前 `ModelEntry` 只存注册时 variant（`model_manager.h:27` "exactly as registered"）、update 直打 HAL 不回写（`:207-209`）、transient 模型不落 platform.db——读回 API 需要 P3 的状态所有权设计先行，此前不可承诺。

## 5. 实施路线（四阶段，每阶段独立可验收）

| 阶段 | 内容 | 验收标准 |
|---|---|---|
| **P1 keypoint decoder 档案** | **前置**：完成 keypoint 两 decoder 的能力矩阵行（facial 零参数入口全硬编码；pose 尺寸键 create_only）；schema 表加两 decoder 及参数键（带 mutability/effect）；向导 keypoint 表单出现"后处理档案"下拉（人脸/姿态，facial 的不可调参数显示为硬编码说明而非表单项）；AMPK metadata 支持 decoder 字段 | 普通用户零手写 JSON 完成姿态模型导入并出 COCO-17 结果；facial 表单不出现任何"填了无效"的键；设备 A/B 验证；SDK 旧路径不受影响 |
| **P2 schema 单源 + CI 同步 + 能力矩阵全表** | schema 源文件 + 构建期生成 C++ 头 + CI 一致性检查；§3.1 矩阵机器可读化进 schema；白名单 4 函数逐一核清参数化/固定入口归属 | 白名单/键表改动单点生效；人为改单侧被 CI 拦截；每键有 mutability/effect/dialect_key；stub 单测覆盖生成表 |
| **P3 统一文档内部表示 + 状态所有权** | 平台内部以统一文档为真相源；**新增状态设计**：desired/effective 双态 + revision、HAL apply 成功→持久化的提交顺序与失败补偿、transient 模型（不落 DB）的内存态读回路径；REST/gRPC 增统一文档入参（旧入参翻译兼容）；方言翻译层泛化全类型 | 旧行/旧包/旧 SDK 调用全兼容；新入参全类型 write→read 往返一致；热更后读回的是 effective 值；重启恢复与失败补偿有负例测试 |
| **P4 SDK v2 与旧面收敛** | SDK 发布统一文档 API（含 `get_postprocess_config`）；`model_variant` 标记 deprecated（双轨至少一个 minor 周期）；raw 模式量化元数据（scale/zero-point）落地；文档/示例迁移 | SDK 测试套件双轨全绿；raw 模型经 SDK 可完成应用侧解码初始化（含量化）；changelog 与迁移指南发布。**金标准对比 2026-09-20 已在 93.72 实测通过**：同 HEF 双注册（typed detection vs untyped raw）+ 同流双订阅按帧对齐，应用侧按官方 NMS 布局解析裸张量 vs 平台 post_result——8 帧 IoU=1.0000、分数差 0.000000、类号/计数全等（bit-exact）；`detection_threshold` 热更新同场验证为已消费键 |

依赖关系：P1 自带前置核查、可先行；P2 前置 = 能力矩阵全表（官方源码核实）；P3 前置 = P2 + 状态所有权设计评审通过；P4 前置 = P3。

**评审状态（2026-09-20）**：评审结论"P1 的 keypoint UI 可独立推进，统一 schema/P3 暂不宜批准"已吸收——P2/P3 以本 v2 补齐的设计（能力矩阵、mutability、状态所有权、张量契约）重新提请评审。

## 6. 兼容矩阵

| 既有面 | 策略 | 保障 |
|---|---|---|
| platform.db 旧行 | 读时翻译为统一文档，写时双轨 | 迁移只读不改写，回滚=旧代码直接可用 |
| AMPK 包 | 旧包按现行合成；新包可带 decoder | unpack 侧版本分支 |
| SDK `register_model(model_type, model_variant)` | 原样接受，内部翻译 | gRPC proto 只增 optional；proto 变更须扫 Go 调用点并重生成 ARM proto |
| gRPC 无 model_type 注册 | **非 transient**：继续接受（映射 `decoder: "raw"`）；**transient**：维持现行 transient gate——无 type 必须显式 `raw_output_only=true` 才放行 | `bundled_package.go:39` 注释明载该 gate；行为零变化 |
| 存量 keypoint 开放 blob（含 HAL 内部键 `yolov8_pose_network_*`、未来插件私有键） | **legacy opaque 通道**（评审 #5）：翻译时未识别键原样保留进 `params.__legacy`，不做逐键校验（延续 09-20 Fix 5 "内容归 HAL 验"的边界）；统一键与 legacy 键冲突时显式报错、统一键优先 | 存量 blob 无损翻译；新键进 schema 后可逐步从 `__legacy` 提升 |
| detection 七键 blob 中的 4 个 advisory 键 | 翻译时原样保留（generic YOLO 全消费，删了反而破坏同构）；schema 标 `effect: advisory` 并注明对白名单函数不生效 | 现有注册零变化；UI 带不生效提示 |
| gRPC RegisterModel 校验 | 校验目标渐进换为统一文档，旧格式先翻译后校验；keypoint blob 走 legacy opaque 不逐键卡 | 2026-09-20 校验器作为兜底继续生效 |
| HAL `HalPostprocessConfig` | 不动；翻译层输出仍是 `config_json` 字符串 | fail-loud/回滚/冒烟链路不变 |
| 厂商插件方言 | 冻结；由翻译层合成；接受层幻影键以 `advisory` 标注而非删除 | 七键完整性由合成器保证 |

## 7. 风险与缓解

| 风险 | 等级 | 缓解 |
|---|---|---|
| schema 过早僵化 | 中 | 键进 schema 源=进版本管理，加键非破坏；P1 只收纳已验证键 |
| 迁移期双表示漂移 | 中 | P2 CI 一致性检查 + P3 统一文档收口；过渡期限定一个版本周期 |
| **能力矩阵与官方库版本漂移**（v2 新增） | 中 | matrix 记录核实基线 1.12.1；vendor 库升级列为重核触发器；`advisory` 判定宁可保守（标 advisory 不影响运行） |
| **热更/读回语义被过度期待**（v2 新增） | 中 | 逐键 mutability + `requires_reload` 显式拒绝；读回 API 在 P3 状态设计落地前不发布 |
| proto 新字段破坏旧客户端 | 低 | 只增 optional；旧字段语义冻结 |
| legacy opaque 通道被滥用为后门 | 低 | `__legacy` 仅 keypoint/明确 grandfather 路径可用；detection 等封闭类型不开放；legacy 键与统一键冲突显式报错 |

## 8. 开放问题

1. CLIP 的 3 个编码器是否开放为用户可选 decoder（涉及 NPU 资源与精度权衡，需设备实测）？
2. OCR/深度/分割等单解码器类型，params 是否全量入 schema（含 `det_map_h/w` 这类强技术键），还是只收高频键、其余留高级逃生口？
3. schema 源格式：手写 JSON vs Go 定义+导出（倾向后者）。
4. decoder 是否上事件总线/订阅结果元数据（消费方想知道结果出自哪个解码器版本）。
5. generic YOLO 函数（七键全消费、含 `anchors` 数组）是否纳入白名单开放？
6. CLIP 分类的 `prompts`/`embedding`/`negatives`/`threshold` 官方经 ZeroMQ 通道传递，`model_manager.h:207` 注释宣称 update 支持 prompts——平台侧通路与方言映射待核，核实结果决定该键在 schema 中的 mutability。

## 9. 与既有工作的关系

- **直接地基**：2026-09-20 后处理整改 7 笔提交——校验/报错/回滚链路全部复用。
- **吸收 P2 暂缓清单**：keypoint pose UI 口（→P1）；Go 过期注释、hal_postprocess.h 死字段清理（→P3 顺带）；SDK 文档（→P4）。
- **本 v2 修订**：2026-09-20 评审 7 条全收——#1 能力矩阵重构（官方 1.12.1 逐文件核实）、#2 schema 加 mutability/effect/dialect_key/clearable/read_source、#3 热更承诺收窄为逐键 + `requires_reload`、#4 P3 补状态所有权与提交协议、#5 legacy opaque 通道、#6 张量契约校验、#7 raw 量化元数据升前置验收。
- **不依赖**：上游 SDK PR 战役、性能套件等并行线，互不阻塞。

---

*本提案为设计文档，未批准实施；实施前须按阶段逐段立项评审。P2/P3 以 v2 设计重新提请评审。*

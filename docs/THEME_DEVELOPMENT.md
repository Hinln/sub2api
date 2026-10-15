# 独立主题层（Theme API v1）

主题功能是此二开仓库的独立扩展，与官方 `.s2plugin` 出站传输插件分开。管理员可以在 **主题管理**（`/admin/themes`）上传主题、查看局部预览、启用主题或恢复官方默认外观。上传本身不会改变已启用的主题。

## 使用与部署

1. 首次部署包含主题层的前后端构建；此后上传和切换主题无需重新构建或重启。
2. 登录管理员后台，打开侧栏“主题管理”。可先下载页面提供的 Ocean 示例包。
3. 上传 `.s2theme` 或 `.zip`，检查名称、版本和预览，再选择“应用主题”。敏感操作沿用管理员二次验证、审计和权限控制。
4. 选择默认主题即可清除覆盖。明暗模式仍使用原有切换按钮，切换主题不会重置明暗选择。

主题是全站设置，覆盖公开页、登录页、用户区及管理员区的标准颜色、字体、圆角和品牌渐变。外部 iframe、图片中的文字/颜色、第三方支付品牌色、业务状态色及直接写在组件中的特殊颜色不属于 v1 token。主题不替换页面结构或业务功能。

当前管理员页面立即应用切换结果；其他已打开页面在重新加载后刷新主题，或距上次请求至少 30 秒时重新获得焦点后刷新。API 超时或失败不会阻止登录：首次加载保留默认外观，已加载的主题保留最近成功状态。主题获取不依赖登录凭据。

## 包格式

包是 ZIP，根目录必须有 `theme.json`，可以附带 `README.md` 和 `LICENSE`；不要再套一层文件夹。包不包含 JavaScript、任意 CSS、HTML、可执行文件、字体或图片。v1 用声明式 token 实现外观，避免主题访问账号数据或绑定上游组件内部结构。

```json
{
  "schema_version": 1,
  "id": "my-brand",
  "name": "My Brand",
  "version": "1.0.0",
  "description": "主题说明",
  "author": "作者",
  "tokens": {
    "colors": { "primary": { "500": "#3b82f6", "600": "#2563eb" } },
    "font": "system",
    "radius_scale": 1.25
  }
}
```

- `schema_version` 固定为 `1`；未知字段、未知版本或非法 token 会拒绝安装。
- `id` 使用小写字母、数字和短横线；`version` 为语义版本。名称、作者和说明有长度限制。
- `colors` 可设置 `primary`、`gray`、`dark`、`accent` 四组调色板；色阶为 `50`、`100`、`200`、`300`、`400`、`500`、`600`、`700`、`800`、`900`、`950`，色值必须为 `#RRGGBB`。
- `primary` 控制主按钮、链接、焦点和品牌效果；`gray` 控制灰阶背景/文字，`dark` 控制暗色表面，`accent` 控制辅助灰蓝色。允许只覆盖部分色阶，其余保留上游默认值。
- `font` 为 `system`、`serif` 或 `mono`，均使用本机字体，不从外网加载字体。
- `radius_scale` 为 `0.25` 至 `2`，缩放标准圆角；完全圆形和无圆角保持原有语义。
- `tokens` 至少包含一个有效配置项。包上限为 256 KiB，清单上限为 32 KiB；服务端校验文件数量、解压大小、CRC、重复路径和链接等。

可复制示例清单 `frontend/themes/examples/ocean/theme.json`，修改后在仓库根目录打包：

```sh
python3 tools/themes/package_theme.py \
  frontend/themes/examples/ocean/theme.json \
  frontend/public/themes/examples/ocean.s2theme
```

打包工具只负责生成可复现归档，完整语义校验由安装接口执行。相同 ZIP 重复上传不会重复安装。包以内容哈希为安装 ID，升级为新的不可变包；上传新版后需明确启用，旧主题保留以便回退。

## 存储与 API

主题使用已有 PostgreSQL `settings` 表，键空间为 `appearance.theme.package.v1.*` 和 `appearance.theme.active.v1`，无需迁移或共享文件卷。包内只保留通过校验的清单，主题随数据库备份迁移。上传不同包分别写入独立键，避免覆盖其他上传。首版不提供删除接口，保留不可变包用于回退。

| 接口 | 权限 | 用途 |
|---|---|---|
| `GET /api/v1/theme` | 无需登录，按 IP 限流 | 仅返回当前主题或 `null` |
| `GET /api/v1/admin/themes` | 管理员 | 当前主题与已安装列表 |
| `POST /api/v1/admin/themes/upload` | 管理员、二次验证 | multipart 字段 `theme` 上传 |
| `PUT /api/v1/admin/themes/active` | 管理员、二次验证 | `{"theme_id":"<安装 ID>"}`；`null` 恢复默认 |

返回沿用项目 `{code, message, data}` 格式。当前主题指针缺失或主题损坏时回退默认；数据库连接等基础设施错误仍返回错误，不伪报切换成功。

## 同步官方更新

主体代码集中在以下独立目录：

- `backend/internal/theme/`：包格式与校验；新增的 service/handler 文件：存储和 HTTP。
- `backend/internal/server/routes/themes.go`：路由；上游 `routes/admin.go` 仅增加一个注册调用。
- `frontend/src/features/themes/`：加载器、管理页、API、类型及测试。
- `frontend/themes/tailwind.mjs`：用 CSS 变量包装上游 Tailwind 默认值；未启用主题时回退到原值。
- 独立中英翻译、示例和打包工具。

同步时主要保留四个前端接入点：`main.ts` 的有界初始化、`tailwind.config.js` 的包装函数、主路由的扩展数组、侧栏菜单条目。业务 Vue 页面不需要逐页打补丁，既有插件、设置 DTO、公开设置注入结构和数据库 schema 均不增加主题字段。

官方增加组件或改变 token 使用方式后，运行主题测试、类型检查和构建，并检查明暗模式与默认主题。合并冲突会减少，但不能保证未来每次升级都无需适配。

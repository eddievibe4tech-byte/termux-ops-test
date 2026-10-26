# termux-ops (`hlops`)

`hlops` 是一個專為 **Android Termux** 設計的備份與維護工具，同時支援一般 Linux 環境。  
它可以幫你備份腳本、設定檔、套件清單與重要資料，並提供磁碟分析、清理與排程輔助。

本專案使用 **Go** 實作，目標是在 Termux 上以單一執行檔完成日常維運。

> **注意**：`hlops` 是純命令列工具（CLI），不是 Android APK，需要先安裝 [Termux](https://termux.dev/)（建議 F-Droid/GitHub 版本）。未來 M10 階段規劃提供 GUI 輔助 App（Companion APK）降低使用門檻，詳見議題 #37。

---

## 設計目標

- **單一執行檔**，適合 Termux 輕量環境
- **多任務備份**：腳本、dotfiles、套件清單可分開備份
- **適應 Termux 儲存模型**：支援 `~/storage/shared` 與 Scoped Storage
- **安全**：備份可驗證、可部分成功、可保留策略
- **排程友好**：整合 `crond` 與 `termux-wake-lock`
- **不做純外部指令包裝器**：動作分 Native / Provider，缺少依賴時優雅跳過

---

## 平台支援

| 平台 | 狀態 | 說明 |
|---|---|---|
| Termux (Android) | **主要支援** | 針對儲存、排程、權限最佳化 |
| 一般 Linux | 支援 | 可在桌面或伺服器使用，Termux 特定功能自動停用 |
| Windows | 暫不支援 | 未來視需求評估 |

---

## 技術選擇

### 主要語言

```text
Go
```

### 原因

- Termux 提供官方 `golang` 套件，可直接編譯。
- 交叉編譯到 `android/arm64` 簡單。
- 標準庫即可處理檔案、壓縮、排程輔助。
- 執行檔無外部依賴，適合手機環境。

---

## 安裝

### 在 Termux 安裝

先更新套件並安裝必要工具：

```bash
pkg update
pkg install golang git termux-api
```

確保已啟用外部儲存權限：

```bash
termux-setup-storage
```

然後安裝 `hlops`：

```bash
go install github.com/yourname/termux-ops/cmd/hlops@latest
```

或從 Release 下載預編譯執行檔：

```bash
# 假設已下載到 ~/Downloads
chmod +x hlops-android-arm64
mv hlops-android-arm64 $PREFIX/bin/hlops
```

---

### 從原始碼編譯（一般 Linux）

```bash
git clone https://github.com/yourname/termux-ops.git
cd termux-ops
go build -o hlops ./cmd/hlops
```

---

## 命令設計

```text
hlops init-config        初始化設定檔
hlops doctor             檢查環境、權限、儲存空間
hlops analyze            分析磁碟與目錄空間
hlops backup             執行所有備份任務
hlops backup --task <n>  執行特定備份任務
hlops cleanup            清理快取與舊備份
hlops run <job>          以鎖檔執行排程任務
hlops schedule generate  產生排程設定（cron）
hlops bridge status      輸出機器可讀狀態 JSON（Companion APK 用）
hlops bridge logs        輸出最新日誌尾端 JSON（Companion APK 用）
hlops version            顯示版本與平台資訊
```

---

## 設定檔

### 路徑

- Termux / Linux 使用者設定：

```text
~/.config/hlops/config.toml
```

- 系統設定（一般 Linux）：

```text
/etc/hlops/config.toml
```

### 設定優先順序

```text
1. 命令列參數
2. 環境變數
3. 使用者設定檔
4. 系統設定檔
5. 內建預設值
```

### 環境變數

```text
HLOPS_CONFIG
HLOPS_STATE_DIR
HLOPS_LOG_DIR
HLOPS_BACKUP_ROOT
HLOPS_NO_COLOR
```

---

## 設定檔範例

```toml
[general]
state_dir = ""
log_dir = ""
report_dir = ""

[analyze]
timeout = "3m"
top_dirs = 10
top_files = 10
min_file_size = "20M"

[backup]
default_destination = "~/storage/shared/hlops-backups"
retention_days = 30
keep = 10
verify = true

# 備份任務一：腳本目錄
[[backup.tasks]]
name = "scripts"
source = "~/scripts"
destination = ""
exclude = [".git", "__pycache__", "*.log"]

# 備份任務二：dotfiles 與設定
[[backup.tasks]]
name = "dotfiles"
source = "~/.config"
destination = ""
include_hidden = true
exclude = ["cache", "Cache", "logs"]

# 備份任務三：Termux 環境資訊
[[backup.tasks]]
name = "termux-env"
type = "termux-env"
destination = ""

[cleanup]
confirm = true
allow_dangerous_actions = false

[cleanup.native.temp]
enabled = true
max_age_days = 7
paths = ["$TMPDIR", "$PREFIX/tmp"]

[cleanup.native.old_backups]
enabled = true

[cleanup.providers.pkg]
enabled = true
```

---

## `hlops doctor`

檢查項目包含：

- 是否已執行 `termux-setup-storage`
- `~/storage/shared` 是否可寫
- 內部儲存與共享儲存剩餘空間
- `crond` 是否安裝並執行
- **電池優化與 OEM 系統背景存活提醒（如 ColorOS / MIUI）**
- 設定檔是否合法

範例：

```bash
hlops doctor
hlops doctor --json
```

---

## `hlops analyze`

```bash
hlops analyze
hlops analyze ~/scripts
hlops analyze --top 20 --min-size 50M
```

- 自動跳過 `/proc`、`/sys` 等虛擬檔案系統。
- 在 Termux 上會忽略 `$PREFIX` 中不該掃描的系統目錄。
- 權限不足時僅記錄，不中斷。

---

## `hlops backup`

### 執行所有任務

```bash
hlops backup --dry-run
hlops backup
```

### 執行特定任務

```bash
hlops backup --task scripts
hlops backup --task termux-env
```

### 備份原則

- 預設格式：`tar.gz`
- 暫存檔與最終檔案同目錄，確保原子 `rename`
- 遇到無法讀取檔案時預設跳過並記錄
- 備份到 `~/storage/shared` 時，因檔案系統不支援 Unix 權限，會額外產生 `manifest.json` 記錄權限
- 每個任務獨立保留策略，也可全域設定
- 產生：

```text
backup archive
backup report (JSON)
sha256 checksum
manifest.json（若目的地不支援權限）
```

### `termux-env` 任務

此任務會自動收集：

```text
pkg list --installed > packages.txt
env | grep -E '^(TERMUX|HOME|PREFIX|TMPDIR)' > termux-env.txt
~/.termux/ 設定檔（若存在）
```

---

## `hlops cleanup`

```bash
hlops cleanup --dry-run
hlops cleanup --yes
```

### 預設清理項目

- `$PREFIX/tmp` 與 `$TMPDIR` 中超過 7 天的檔案
- 舊備份（依保留策略）
- `pkg` / `apt` 快取（若可用）

### 風險分級

```text
safe
moderate
dangerous
```

危險動作需同時指定：

```bash
hlops cleanup --yes --dangerous
```

---

## `hlops run`

供排程使用，內部會：

- 讀取設定檔
- 使用鎖檔避免重複執行
- 呼叫 `termux-wake-lock`（若在 Termux）
- 寫入日誌與報告

範例：

```bash
hlops run backup
hlops run cleanup
```

---

## `hlops bridge`

提供未來 **Companion APK**（議題 #37/#38）消费的穩定機器可讀 JSON，
完整契約見 [docs/companion-app-bridge.md](docs/companion-app-bridge.md)。

```bash
hlops bridge status              # 最新工作報表的正規化狀態（schema v1）
hlops bridge status --job backup # 限定工作種類
hlops bridge logs --lines 20     # 主日誌最後 20 行
```

特色：唯讀、無副作用；尚無報表時回傳 `"status": "no_report"` 的合法 JSON 而非錯誤。

---

## `hlops schedule generate`

Termux 上建議使用 `crond` 排程。

### 產生 cron 設定

```bash
hlops schedule generate cron --job backup --time "0 2 * * *"
```

輸出範例：

```cron
0 2 * * * /data/data/com.termux/files/usr/bin/termux-wake-lock && /data/data/com.termux/files/home/go/bin/hlops run backup; /data/data/com.termux/files/usr/bin/termux-wake-unlock
```

### 安裝步驟

1. 安裝 `cronie`：

```bash
pkg install cronie
```

2. 將輸出加入 `crontab`：

```bash
crontab -e
```

3. 啟動 `crond`：

```bash
crond
```

4. 建議將 Termux 加入 Android 電池優化白名單。

---

## 退出碼

| 退出碼 | 意義 |
|---:|---|
| 0 | 成功，或可選依賴不存在而跳過 |
| 1 | 一般錯誤 |
| 2 | 設定錯誤 |
| 3 | 權限不足（如未執行 `termux-setup-storage`） |
| 4 | 部分成功（部分檔案跳過） |
| 5 | 使用者明確指定的依賴不存在 |
| 6 | 鎖檔被持有，另一個實例正在執行 |

---

## Termux 特別注意事項

### 1. 儲存權限

首次使用請執行：

```bash
termux-setup-storage
```

否則無法備份到 `~/storage/shared`。

### 2. 電池最佳化與背景防殺 (特別是 OPPO / 小米 / vivo)

Android 系統（特別是 **ColorOS、HyperOS/MIUI、OriginOS** 等客製化 ROM）有極度激進的背景清理機制，會直接忽略 WakeLock 強制終止 Termux。若要讓排程備份穩定運行，**必須手動完成以下設定**：

1. **電池最佳化豁免**：
   - 進入「設定 → 應用程式 → Termux → 耗電量管理」，改為 **「允許完全背景行為」** 或 **「不最佳化」**。
2. **多工介面鎖定**：
   - 開啟多工切換畫面（Recents），找到 Termux 卡片並點擊選單加上 **「鎖定（小鎖頭）」**，防止一鍵清理時被終止。
3. **允許自啟動**：
   - 進入「設定 → 應用程式 → 自啟動管理」，允許 Termux 自啟動。
4. **關閉夜間睡眠最佳化**（排程設定於半夜時必做）：
   - 進入「設定 → 電池 → 更多/進階設定」，關閉 **「睡眠待機最佳化」**，避免系統在凌晨切斷 CPU 與網路資源。

> 更多不同品牌手機的設定指引，請參考 [Don't kill my app!](https://dontkillmyapp.com/)。

### 3. 背景任務鎖

排程執行備份時，工具會自動呼叫：

```bash
termux-wake-lock
```

避免 CPU 休眠導致備份中斷。

### 4. `/sdcard` 權限限制

`~/storage/shared` 通常使用 FUSE，不支援：

- 執行檔位元
- 完整 Unix 權限
- 符號連結

因此備份時會：

- 將權限資訊存入 `manifest.json`
- 恢復時建議使用 `hlops restore`（未來功能）或手動 `chmod`

### 5. 大型備份

手機 I/O 與電池有限，建議：

- 排除大型快取目錄
- 備份到外部儲存或雲端（未來）
- 使用增量備份（未來功能）

---

## 專案結構

```text
termux-ops/
├── cmd/
│   └── hlops/
│       └── main.go
├── internal/
│   ├── cli/
│   ├── config/
│   ├── platform/
│   │   ├── termux.go
│   │   └── linux.go
│   ├── analyze/
│   ├── backup/
│   │   ├── task.go
│   │   ├── tarball.go
│   │   └── manifest.go
│   ├── cleanup/
│   │   ├── native/
│   │   └── providers/
│   ├── run/
│   ├── schedulegen/
│   ├── lock/
│   ├── report/
│   └── logging/
├── packaging/
│   └── termux/
│       └── install.sh
├── docs/
│   ├── termux-quickstart.md
│   └── restore-guide.md
├── tests/
└── README.md
```

---

## 路線圖

### v0.1

- Go 專案骨架
- `hlops version`
- 設定檔讀取
- `hlops doctor`
- Termux 路徑偵測

### v0.2

- `hlops analyze`
- Termux 儲存空間分析
- 跳過虛擬檔案系統

### v0.3

- 多任務備份
- `tar.gz` 原子寫入
- `manifest.json` 權限記錄
- 套件清單與環境備份

### v0.4

- Native cleanup
- Termux 暫存清理
- 舊備份保留策略

### v0.5

- Provider cleanup
- `pkg` / `apt` 快取
- probe / skipped 狀態

### v0.6

- `hlops run`
- lockfile
- `termux-wake-lock` 整合
- `schedule generate cron`

### v1.0

- Termux 完整驗收
- 安裝腳本
- 文件完善
- Release 打包（`android/arm64`）

---

## 未來功能

- 增量備份 / 快照
- 備份加密
- 遠端備份（rsync / rclone / S3）
- `hlops restore` 自動恢復
- `termux-job-scheduler` 整合
- TUI 介面

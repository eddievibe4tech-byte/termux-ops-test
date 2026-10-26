# Companion APK Bridge Contract（M10-1，議題 #37 / #38）

本文定義 `hlops` CLI 與未來 Android **Companion APK** 之間的機器可讀介面契約。
APK **不解析人類可讀輸出或日誌檔**，只消費本文件描述的穩定 JSON。

## 設計原則

1. **Schema 版本化**：所有 bridge 輸出都帶有 `"schema": 1` 欄位。APK 讀取後先檢查此欄位；
   欄位新增採「向後相容」原則（APK 必須忽略未知欄位），破壞性變更才會 bump schema。
2. **單一資料來源**：正規化邏輯集中在 Go 套件 `internal/bridge`，CLI 端與未來任何
   Intent/ContentProvider 共用同一份程式碼，避免雙份實作漂移。
3. **無副作用、可離線**：bridge 命令唯讀，不啟動工作、不寫入狀態；報表缺失時仍回傳
   合法 JSON（HTTP-200 語意），APK 不需要處理「命令失敗＝沒有資料」的歧義。

## 資料流

```text
crond / hlops run backup ──寫入──▶ $HOME/.local/state/hlops/reports/*.json
                                        │
hlops bridge status  ◀──正規化──────────┤  (internal/bridge)
hlops bridge logs    ◀──tail────────────┘  $HOME/.local/state/hlops/logs/hlops.log
```

## 命令

### `hlops bridge status [--job backup|cleanup]`

回傳最新一份工作報表的正規化狀態。預設取「任意工作中最新」的一份；
`--job` 可限定工作種類。尚無報表時回傳 `status: "no_report"` 文件（非錯誤）。

### `hlops bridge logs [--lines N]`

回傳主日誌檔最後 N 行（預設 50），以 JSON 字串陣列呈現，方便 APK 直接渲染。
實作上以串流方式（`bufio.Scanner` + 滑動視窗）讀取，記憶體佔用恆定為
`O(N × 平均行長)`，即使日誌累積數十 MB 也不會造成 Termux OOM。

### `hlops run <job> --json --bridge`

執行工作並以**單一 BridgeStatus JSON 文件**輸出結果（契約與
`hlops bridge status` 完全相同）。若工作在寫出報表前就硬失敗
（lock 衝突、設定錯誤等），stdout 仍會輸出一份
`status: "failed"` 且帶 `error` 欄位的合法文件；APK **必須同時**
檢查非零 exit code——非零代表嚴重錯誤，不得改讀舊報表當成好結果。
（注意：`hlops backup --json` 維持輸出 raw report 格式，不受影響。）

## BridgeStatus JSON Schema（v1）

```json
{
  "schema": 1,
  "generated_at": "2026-01-01T02:00:05Z",
  "job": "backup",
  "source_file": "/data/data/com.termux/files/home/.local/state/hlops/reports/backup-20260101T020000Z.json",
  "status": "ok",
  "started": "2026-01-01T02:00:00Z",
  "finished": "2026-01-01T02:00:05Z",
  "dry_run": false,
  "success": 3,
  "failed": 0,
  "skipped": 0,
  "tasks": [
    {
      "name": "scripts",
      "status": "success",
      "archive": "~/backups/scripts-20260101.tar.gz",
      "sha256": "ab12...",
      "size_bytes": 1048576
    }
  ],
  "hlops_version": "0.4.0"
}
```

### 頂層欄位

| 欄位 | 型別 | 說明 |
|---|---|---|
| `schema` | int | 契約版本，目前為 `1` |
| `generated_at` | string (RFC3339 UTC) | bridge 產生此文件的時間 |
| `job` | string | `backup` \| `cleanup` |
| `source_file` | string | 來源報表絕對路徑（除錯用） |
| `status` | string | 聚合結果，見下方狀態表 |
| `started` / `finished` | string | 工作起訖時間（來自原始報表） |
| `dry_run` | bool | 是否為预览執行 |
| `success` / `failed` / `skipped` | int | 各任務計數 |
| `tasks[]` | array | 正規化的逐項結果（保證為陣列，絕不為 null） |
| `error` | string | 僅在硬失敗且未寫出任何報表時出現（頂層錯誤訊息） |
| `hlops_version` | string | 產生報表的 CLI 版本 |

排序說明：「最新報表」一律以檔案系統 `ModTime` 判定，不从檔名解析時間戳記，
因此任務名稱含 dash（如 `backup-website-blog-...json`）或未來檔名格式變更
都不影響正確性。空任務清單的已存在報表聚合為 `ok`；`no_report` 僅代表
「找不到任何報表檔」。

### 聚合 `status` 值

| 值 | 意義 | APK UI 建議 |
|---|---|---|
| `ok` | 全部成功 | 綠 |
| `partial` | 混合 success/skipped 或 dry-run，無失敗 | 黃 |
| `failed` | 至少一項失敗 | 紅 |
| `no_report` | 尚無任何報表（新安裝） | 灰色引導頁 |

### `tasks[].status` 值

`success` \| `failed` \| `skipped` \| `dry-run` \| `done`（cleanup 類任務完成）。
未認識的值 APK 應以中性樣式顯示，不得當做錯誤。

## 相容性規則

- **CLI → APK**：新增欄位不 bump schema；APK 必須忽略未知欄位。
- **APK → CLI**：APK 若遇到 `schema > 自身支援版本`，應降級為「請更新 hlops」提示，而非崩潰。
- 報表目錄不存在、日誌檔不存在皆不是錯誤情境，bridge 一律回傳合法 JSON。

## 後續里程碑（不在 M10-1 範圍）

- M10-2：Companion APK 原型，透過 `termux-exec`/`am start` intent 呼叫 `hlops bridge ...`。
- M10-3：考慮以 Termux:API 或本地 HTTP sidecar 取代每次 spawn CLI 的方式。

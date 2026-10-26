---
name: go_android_termux_reviewer
description: 專門審查與修正針對 Android/Termux 環境開發的 Go CLI 程式碼。重點聚焦於隱藏的 Go 陷阱（NBSP、變數遮蔽、Scanner 限制、切片搬移效能）以及 Android 14 / OEM 特製系統（ColorOS、HyperOS）的背景防殺、屬性探測與儲存沙盒邊界。
---

# Go (Android / Termux / OEM) 專案 Code Review 規範

當使用者要求 Review、優化或撰寫運行在 Android / Termux 環境下的 Go 程式碼（特別是涉及 CLI、背景任務、日誌處理、OEM 識別、Companion APK 跨行程通訊）時，必須主動依照以下四個維度逐一核對並修正。

---

## 檢查維度一：Go 語言隱形地雷與語法相容性

### 1. 隱形非中斷空白（NBSP `\u00a0`）
- **痛點**：從網頁、IDE 格式化或 Slack/Discord 複製程式碼時，常夾帶 Unicode `\u00a0`。Go 編譯器（`gc`）對此零容忍，會直接中斷編譯報錯 `illegal character U+00A0`。
- **審查要求**：
  - 檢查常數賦值、結構體標籤（Struct Tags）、Cobra 指令定義處是否存在非標準空白。
  - 所有排版空白必須純化為 ASCII 空格（`\x20`）。

### 2. 變數遮蔽（Variable Shadowing）
- **痛點**：外層已宣告變數（如 `var lines int`），內層卻使用短變數宣告 `lines, err := ...`，導致型別被覆寫為切片，引發靜態檢查警告與型別邏輯混亂。
- **審查要求**：
  - 檢查函式內部的 `:=` 是否意外覆蓋了外層參數或閉包變數。
  - 重新命名內部局部變數（例如將 `lines` 改為 `maxLines` 或 `tailLines`）。

### 3. `sync.Once` 與快取並行安全
- **痛點**：宣告 `ResetCache()` 直接對全域 `sync.Once` 重新賦值（`once = sync.Once{}`），在並行單元測試（`t.Parallel()`）下會引發嚴重的 Data Race。
- **審查要求**：
  - 純業務邏輯（如解析、分類）必須拆解為不依賴全域狀態的純函數（Pure Function），以便單元測試直接傳入 Mock 資料。
  - 若需要快取重置，應使用 `sync.RWMutex` 或原子指標（`atomic.Pointer`），而非直接重置 `sync.Once`。

---

## 檢查維度二：Go 標準庫在大型資料下的效能陷阱

### 1. `bufio.Scanner` 的 64KB 硬上限
- **痛點**：`bufio.NewScanner` 預設的緩衝區上限為 `bufio.MaxScanTokenSize`（64KB）。一旦 Log 中包含大型 JSON Dump、Base64 字串或 Panic Stacktrace，讀取超過 64KB 的單行會立即噴出 `bufio.ErrTooLong` 導致程序崩潰。
- **審查要求**：
  - 使用 `scanner.Buffer(buf, maxCapacity)` 明確放寬單行上限（建議至少 1MB）。
  - 若檔案極大或無法預期行長，應改用 `bufio.Reader.ReadLine()` 或反向讀取（Reverse Seek）。

### 2. 切片滑動視窗的記憶體搬移效能（$O(N \times M)$）
- **痛點**：實作 `tail` 邏輯時，每讀入一行就執行 `copy(lines, lines[1:])`，當檔案有 10 萬行且保留 200 行時，會產生近 10 萬次記憶體搬移與 GC 負擔。
- **審查要求**：
  - **強制要求**：必須改用 **環形緩衝區（Ring Buffer）**，利用 `ring[totalLines % maxLines]` 存放，最後再重組順序，將複雜度壓回純粹的 $O(N)$。

```go
// 規範範例：Ring Buffer 實作 tail
ring := make([]string, maxLines)
totalLines := 0
for scanner.Scan() {
    ring[totalLines%maxLines] = scanner.Text()
    totalLines++
}
```

---

## 檢查維度三：Android 系統屬性（getprop）與 OEM 識別精準度

### 1. 歐加系（ColorOS / Realme UI / OxygenOS）屬性更新

- **痛點**：誤寫或漏掉核心屬性。自 Android 11 起，OPPO、OnePlus、realme 核心代碼統一整合成「歐加（Oplus）」，舊屬性 `opporom` 在部分新機型上可能缺席。
- **審查要求**：
  - 必須同時檢查：`ro.build.version.oplusrom`、`ro.build.version.opporom`、`ro.build.version.realmeui`。
  - 嚴禁誤植類似 `riscoversion` 等無關字串。

### 2. 小米與榮耀獨立系統屬性

- **痛點**：HyperOS（澎湃 OS）與獨立後的榮耀（MagicOS）具有專屬標識，若只檢查 `ro.miui.ui.version.name` 或 `emui` 會導致漏判。
- **審查要求**：
  - 小米體系：必須補齊 `ro.mi.os.version.name`（HyperOS 專用）。
  - 華為/榮耀：必須補齊 `ro.build.version.magic`（MagicOS 專用）。

### 3. Manufacturer 與 Brand 必須雙重比對

- **痛點**：子品牌（POCO、iQOO、realme、Redmi、Honor）的底層屬性中，`ro.product.manufacturer` 常填寫母公司，而 `ro.product.brand` 才是該子品牌；反之亦然。
- **審查要求**：
  - 比對 OEM 時，邏輯必須涵蓋 `Manufacturer` 或 `Brand` 任一符合。
  - 保留提取時原始大小寫以供 UI 呈現，比對時使用大小寫無關（`strings.EqualFold`）方式檢查。

### 4. `exec.Command` 在精簡環境與逾時防護

- **痛點**：在 Termux 背景 daemon、crond 或無前台 context 下，環境變數 `PATH` 可能不包含 `/system/bin`；且直接執行 `getprop` 若遭遇 Binder 服務卡死會導致程序永久 Hang。
- **審查要求**：
  - 優先檢查 `/system/bin/getprop`。
  - 一律使用 `exec.CommandContext` 加上 2~3 秒的超時保護。

---

## 檢查維度四：OPPO (ColorOS 14 / Android 14) 實機環境防護

針對 OPPO A5 Pro 等中階聯發科（MediaTek）搭載 ColorOS 14+ 的實體裝置，需額外檢查以下項目：

### 1. 背景 I/O 峰值與 Athena 調度殺手

- **痛點**：ColorOS 14 的 Athena 資源管理機制對背景行程極度敏感。若 Termux 在背景為讀取幾十 MB 的 Log 產生持續數秒的高 I/O 與 CPU 解碼峰值，系統會立即判定為異常耗電並觸發 `SIGSTOP`（凍結）或 `SIGKILL`。
- **審查要求**：
  - 避免對大型記錄檔進行無意義的全文循序掃描。
  - CLI 工具應設計截斷限制，並建議使用者在 ColorOS「設定 → 電池 → 更多設定」關閉「夜間深度休眠」，並在「耗電管理」開啟「允許完全背景行為」。

### 2. 跨 App 儲存隔離（Scoped Storage 邊界）

- **痛點**：Termux 的內部目錄 `/data/data/com.termux/files/...` 屬於私人沙盒。無論外部的 Companion APK 宣告了什麼權限（即使宣告了 `MANAGE_EXTERNAL_STORAGE`），Android 14 仍嚴格禁止其直接以檔案路徑跨 App 讀取。
- **審查要求**：
  - `paths` 指令或 Bridge API 輸出的路徑若位於內部私有空間，必須在註解或設計規範中標明：**Companion APK 只能透過 Termux 的 `RUN_COMMAND`（Intent）接收 CLI 的標準輸出 JSON，不可在 Companion APK 內部直接嘗試開啟該路徑**。

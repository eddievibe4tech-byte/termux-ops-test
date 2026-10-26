# hlops Termux 快速上手

> **定位說明**：`hlops` 為純命令列工具（CLI），需在 Termux 環境下執行，**並非 Android APK**。
> 未來版本（M10）規劃提供 GUI 輔助 App（Companion APK），用於一鍵權限設定、Termux 檢查與備份觸發，詳見議題 #37。

## 安裝

### 方式一：安裝腳本（從 GitHub Release）
```bash
bash <(curl -sSL https://raw.githubusercontent.com/eddievibe4tech-byte/termux-ops-test/main/scripts/install-termux.sh)
```

### 方式二：從來源碼建置
```bash
pkg install golang git -y
git clone https://github.com/eddievibe4tech-byte/termux-ops-test
cd termux-ops-test
CGO_ENABLED=0 go build -o $PREFIX/bin/hlops ./cmd/hlops
```

## 必要前置作業
```bash
termux-setup-storage   # 授予共享儲存權限（~/storage/shared）
```

## 初始化設定
```bash
hlops init-config      # 產生 scripts / dotfiles / termux-env 三個範例任務
hlops doctor           # 檢查環境與設定
hlops analyze          # 掃描儲存空間
```

## 備份與清理
```bash
hlops run backup               # 執行所有備份任務
hlops run backup --task scripts
hlops run cleanup              # 執行安全清理動作
hlops run cleanup --dry-run
```

## 排程（crond）
1. 安裝 cronie：
   ```bash
   pkg install cronie termux-services -y
   ```
2. 啟動 crond：
   ```bash
   sv-enable crond
   # 或手動：crond
   ```
3. 產生 cron 行並加入 crontab：
   ```bash
   hlops schedule generate cron --time "0 2 * * *" --job backup >> ~/.config/cron/crontabs/$USER
   crontab ~/.config/cron/crontabs/$USER   # Termux cronie 位置
   ```

## 電池優化白名單與背景防殺（特別是 OPPO / 小米 / vivo）
Android 系統（特別是 **ColorOS、HyperOS/MIUI、OriginOS** 等客製化 ROM）有極度激進的背景清理機制，會直接忽略 WakeLock 強制終止 Termux。若要讓排程備份穩定運行，**必須手動完成以下設定**：

1. **電池最佳化豁免**：
   - 進入「設定 → 應用程式 → Termux → 耗電量管理」，改為 **「允許完全背景行為」** 或 **「不最佳化」**。
   - 系統設定 → App → Termux → 電池 → unrestricted / 不限制
2. **多工介面鎖定**：
   - 開啟多工切換畫面（Recents），找到 Termux 卡片並點擊選單加上 **「鎖定（小鎖頭）」**，防止一鍵清理時被終止。
3. **允許自啟動**：
   - 進入「設定 → 應用程式 → 自啟動管理」，允許 Termux 自啟動。
4. **關閉夜間睡眠最佳化**（排程設定於半夜時必做）：
   - 進入「設定 → 電池 → 更多/進階設定」，關閉 **「睡眠待機最佳化」**，避免系統在凌晨切斷 CPU 與網路資源。

另外可在 Termux 中執行 `termux-wake-lock`（hlops run 會自動處理）。

**自動偵測**：在 Termux 中執行 `hlops doctor`，hlops 會透過 `getprop` 辨識裝置是否屬於 ColorOS / HyperOS-MIUI / OriginOS / EMUI / Flyme 等激進省電 ROM，並直接輸出對應品牌的防殺 checklist，無需逐項比對手冊。若排程備份「沒有留下任何報告」，`hlops run --json` 與異常退出診斷也會提示最可能原因（被系統终止 vs. 真正 crash）。

> 更多不同品牌手機的設定指引，請參考 [Don't kill my app!](https://dontkillmyapp.com/)。

## 日誌
預設寫入 `$HOME/.local/state/hlops/logs`。

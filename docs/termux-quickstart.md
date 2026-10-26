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

## 電池優化白名單
Android 可能終止背景工作。請將 Termux 加入電池最佳化豁免：
- 系統設定 → App → Termux → 電池 →  unrestricted / 不限制
- 或在 Termux 中執行 `termux-wake-lock`（hlops run 會自動處理）。

## 日誌
預設寫入 `$HOME/.local/state/hlops/logs`。

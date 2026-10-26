# hlops 還原指南

## 1. 找到備份檔
備份輸出於設定中的 `destination`，命名格式：`<task>-YYYYMMDD-HHMMSS.tar.gz`，
每個備份都有對應 `.sha256` 校验檔。

```bash
sha256sum -c scripts-20261026-020000.tar.gz.sha256
```

## 2. 還原 tar.gz 任務
```bash
tar -xzf scripts-20261026-020000.tar.gz -C ~/
```

### 共享儲存（/sdcard）備份
Android 共享儲存不支援 Unix 權限，hlops 會在 tar 旁寫入 `manifest.json`
記錄每個檔案的 mode/uid/gid。內部儲存還原後可依 manifest 重設權限：
```bash
# 檢視 manifest
cat manifest.json | python -m json.tool
# 手動 chmod 範例
chmod 700 ~/.ssh/id_ed25519
```

## 3. 還原 termux-env 任務
```bash
tar -xzf termux-env-*.tar.gz -C ~/
# 重新安裝套件清單
while read -r pkg; do pkg install -y "$pkg"; done < termux-packages.txt
```

## 4. 完整重建流程（新裝置）
1. 安裝 Termux（F-Droid 版本）。
2. `termux-setup-storage`
3. 安裝 hlops（見 quickstart）。
4. 複製備份到裝置、逐一解壓還原。
5. `hlops doctor` 驗證環境。

## 故障排除
- **儲存權限錯誤**：重新執行 `termux-setup-storage`。
- **crond 未觸發**：確認 Termux 未被電池最佳化中止；使用 `sv-enable crond`。
- **lock 殘留**：刪除 `$HOME/.local/state/hlops/locks/*.lock`。

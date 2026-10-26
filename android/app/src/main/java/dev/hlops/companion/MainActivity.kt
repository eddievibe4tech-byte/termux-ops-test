package dev.hlops.companion

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.provider.Settings
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

// MainActivity is the whole Companion UI (issue #37):
//   1. detect Termux install, link to F-Droid when missing
//   2. onboarding helpers (battery optimisation whitelist, storage)
//   3. "立即備份" -> launch Termux with `hlops run backup --json --bridge`
//   4. "查看日誌/狀態" -> read bridge JSON via SAF (no MANAGE_EXTERNAL_STORAGE)
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent { MaterialTheme { HlopsScreen() } }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HlopsScreen() {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var statusText by remember { mutableStateOf<String?>(null) }
    var logText by remember { mutableStateOf<String?>(null) }

    // SAF picker: users grant read access to ~/.local/state/hlops/reports and
    // .../logs once; persistable URIs survive reboots. This replaces the
    // rejected MANAGE_EXTERNAL_STORAGE route (Google Play policy, issue #37).
    val openStatus = rememberLauncher { uri ->
        scope.launch { statusText = readBridgeStatus(context, uri) }
    }
    val openLog = rememberLauncher { uri ->
        scope.launch { logText = readLogFile(context, uri) }
    }

    Scaffold(topBar = { TopAppBar(title = { Text("hlops Companion") }) }) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(16.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            if (!TermuxBridge.isTermuxInstalled(context)) {
                Card(Modifier.fillMaxWidth()) {
                    Column(Modifier.padding(16.dp), Arrangement.spacedBy(8.dp)) {
                        Text("未偵測到 Termux", style = MaterialTheme.typography.titleMedium)
                        Text("hlops 核心邏輯執行在 Termux 內，請先從 F-Droid 安裝 Termux（勿用 Google Play 舊版）。")
                        Button(onClick = {
                            openUrl(context, "https://f-droid.org/packages/com.termux/")
                        }) { Text("前往 F-Droid 安裝 Termux") }
                    }
                }
            } else {
                Card(Modifier.fillMaxWidth()) {
                    Column(Modifier.padding(16.dp), Arrangement.spacedBy(8.dp)) {
                        Text("Termux 已安裝 ✓", style = MaterialTheme.typography.titleMedium)
                        Text("電池優化與自啟動引導：ColorOS / MIUI 等系統請將 Termux 加入白名單，" +
                            "並執行 termux-wake-lock。詳見 README「背景存活」章節。")
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            OutlinedButton(onClick = {
                                openBatterySettings(context)
                            }) { Text("電池最佳化") }
                            OutlinedButton(onClick = { TermuxBridge.launchTermuxSettings(context) }) {
                                Text("Termux 設定")
                            }
                        }
                    }
                }

                Button(
                    onClick = { TermuxBridge.launchRunBackup(context, "backup") },
                    modifier = Modifier.fillMaxWidth()
                ) { Text("立即備份（喚起 Termux 執行 hlops run backup）") }

                OutlinedButton(
                    onClick = { openStatus.launch(arrayOf("application/json", "*/*")) },
                    modifier = Modifier.fillMaxWidth()
                ) { Text("讀取最新 bridge 狀態報表") }

                statusText?.let { txt ->
                    Card(Modifier.fillMaxWidth()) {
                        Text(
                            txt,
                            modifier = Modifier.padding(12.dp),
                            fontFamily = FontFamily.Monospace,
                            style = MaterialTheme.typography.bodySmall
                        )
                    }
                }

                OutlinedButton(
                    onClick = { openLog.launch(arrayOf("text/plain", "application/octet-stream", "*/*")) },
                    modifier = Modifier.fillMaxWidth()
                ) { Text("查看日誌尾端") }

                logText?.let { txt ->
                    Card(Modifier.fillMaxWidth()) {
                        Text(
                            txt,
                            modifier = Modifier
                                .padding(12.dp)
                                .heightIn(max = 320.dp),
                            fontFamily = FontFamily.Monospace,
                            style = MaterialTheme.typography.bodySmall
                        )
                    }
                }
            }
        }
    }
}

// Must be @Composable: it wraps rememberLauncherForActivityResult, which is a
// @Composable function (Kotlin requires the caller to be composable as well).
@Composable
private fun rememberLauncher(
    onResult: (Uri) -> Unit
) = androidx.activity.compose.rememberLauncherForActivityResult(
    ActivityResultContracts.OpenDocument()
) { uri -> uri?.let(onResult) }

private suspend fun readBridgeStatus(context: Context, uri: Uri): String =
    withContext(Dispatchers.IO) {
        // inner runCatching returns Result<String>; `?:` inside it must not
        // return from the enclosing function, so we spell the null case out.
        runCatching {
            val raw = context.contentResolver.openInputStream(uri)?.use { it.readBytes().decodeToString() }
                ?: error("無法開啟檔案")
            val parsed = BridgeStatus.fromJson(raw)
            parsed.fold(
                onFailure = { "解析失敗：${it.message}\n\n原始內容：\n$raw" },
                onSuccess = { s ->
                    buildString {
                        appendLine("job=${s.job}  status=${s.status}")
                        appendLine("success=${s.success} failed=${s.failed} skipped=${s.skipped}")
                        s.error?.let { appendLine("error=$it") }
                        appendLine()
                        s.tasks.forEach { t ->
                            appendLine("- ${t.name}: ${t.status}" +
                                (t.archive?.let { " ($it)" } ?: "") +
                                (t.error?.let { " ! $it" } ?: ""))
                        }
                    }
                }
            )
        }.getOrElse { "讀取失敗：${it.message}" }
    }

private suspend fun readLogFile(context: Context, uri: Uri): String =
    withContext(Dispatchers.IO) {
        runCatching {
            val lines = context.contentResolver.openInputStream(uri)?.bufferedReader()?.use { reader ->
                // constant-memory tail: keep last 50 lines in a ring buffer
                val window = ArrayDeque<String>()
                for (line in reader.lines()) {
                    window.addLast(line)
                    if (window.size > 50) window.removeFirst()
                }
                window.toList()
            } ?: emptyList()
            lines.joinToString("\n")
        }.getOrElse { "讀取失敗：${it.message}" }
    }

private fun openBatterySettings(context: Context) {
    // Best-effort: OEM screens differ heavily (issue #43 doc note); fall back
    // to the global battery-optimisation list which exists since API 23.
    val intent = Intent(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS)
    runCatching { context.startActivity(intent) }
        .onFailure {
            openUrl(context, "https://dontkillmyapp.com/")
        }
}

private fun openUrl(context: Context, url: String) {
    runCatching {
        context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
    }
}

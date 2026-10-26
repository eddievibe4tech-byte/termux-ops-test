package dev.hlops.companion

import android.content.ComponentName
import android.content.Context
import android.content.Intent

// TermuxBridge builds the intents that drive `hlops bridge ...` inside Termux
// (issue #37; contract in docs/companion-app-bridge.md).
//
// Mechanism: Termux's `.app.TermuxActivity` accepts an EXTRA_CMD intent and
// runs the command in a fresh session — the same trick termux-am / Termux:GUI
// use. We deliberately do NOT embed backup/cleanup logic here; the APK only
// launches commands and later renders the JSON files they leave behind.
object TermuxBridge {
    const val TERMUX_PACKAGE = "com.termux"
    private const val TERMUX_ACTIVITY = "com.termux.app.TermuxActivity"
    private const val EXTRA_CMD = "com.termux.intent.EXTRA_CMD"

    /** True when Termux is installed (Android 11+ needs the <queries> entry). */
    fun isTermuxInstalled(context: Context): Boolean = try {
        context.packageManager.getPackageInfo(TERMUX_PACKAGE, 0)
        true
    } catch (_: Exception) {
        false
    }

    /** Launch Termux running `hlops run <job> --json --bridge` (issue #37 AC: 立即備份). */
    fun launchRunBackup(context: Context, job: String = "backup") {
        context.startActivity(runCommandIntent("hlops run $job --json --bridge"))
    }

    /** Launch Termux running an arbitrary hlops bridge command for diagnostics. */
    fun launchBridgeStatus(context: Context) {
        context.startActivity(runCommandIntent("hlops bridge status"))
    }

    /** Deep-link into Termux settings (used by the onboarding wizard). */
    fun launchTermuxSettings(context: Context) {
        val i = Intent("com.termux.APPLICATION_PREFERENCES").apply {
            setClassName(TERMUX_PACKAGE, "com.termux.app.SettingsActivity")
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        }
        context.startActivity(i)
    }

    internal fun runCommandIntent(cmd: String): Intent = Intent().apply {
        component = ComponentName(TERMUX_PACKAGE, TERMUX_ACTIVITY)
        putExtra(EXTRA_CMD, cmd)
        addFlags(
            Intent.FLAG_ACTIVITY_NEW_TASK or
                Intent.FLAG_ACTIVITY_CLEAR_TOP or
                Intent.FLAG_ACTIVITY_SINGLE_TOP
        )
    }
}

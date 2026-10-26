package dev.hlops.companion

import org.json.JSONObject

// BridgeStatus mirrors the v1 JSON contract in docs/companion-app-bridge.md.
// Kept as a plain parse of org.json (no third-party deps) so the schema is
// reviewable line-by-line against the Go struct in internal/bridge/status.go.
data class TaskState(
    val name: String,
    val status: String, // success|failed|skipped|dry-run|done
    val archive: String?,
    val sha256: String?,
    val sizeBytes: Long,
    val freedBytes: Long,
    val error: String?
)

data class BridgeStatus(
    val schema: Int,
    val generatedAt: String,
    val job: String,
    val sourceFile: String?, // debug-only absolute path of the report on disk
    val status: String, // ok|partial|failed|no_report
    val started: String?,
    val finished: String?,
    val dryRun: Boolean,
    val success: Int,
    val failed: Int,
    val skipped: Int,
    val tasks: List<TaskState>,
    val error: String?,
    val hlopsVersion: String?
) {
    companion object {
        const val SUPPORTED_SCHEMA = 1

        fun fromJson(raw: String): Result<BridgeStatus> = runCatching {
            val o = JSONObject(raw)
            val schema = o.optInt("schema", -1)
            require(schema <= SUPPORTED_SCHEMA) {
                "bridge schema $schema newer than app support ($SUPPORTED_SCHEMA); please update hlops"
            }
            val tasksArr = o.optJSONArray("tasks")
            val tasks = buildList {
                if (tasksArr != null) {
                    for (i in 0 until tasksArr.length()) {
                        val t = tasksArr.getJSONObject(i)
                        add(
                            TaskState(
                                name = t.optString("name"),
                                status = t.optString("status"),
                                archive = t.optString("archive").ifEmpty { null },
                                sha256 = t.optString("sha256").ifEmpty { null },
                                sizeBytes = t.optLong("size_bytes"),
                                freedBytes = t.optLong("freed_bytes"),
                                error = t.optString("error").ifEmpty { null }
                            )
                        )
                    }
                }
            }
            BridgeStatus(
                schema = schema,
                generatedAt = o.optString("generated_at"),
                job = o.optString("job"),
                sourceFile = o.optString("source_file").ifEmpty { null },
                status = o.optString("status"),
                started = o.optString("started").ifEmpty { null },
                finished = o.optString("finished").ifEmpty { null },
                dryRun = o.optBoolean("dry_run"),
                success = o.optInt("success"),
                failed = o.optInt("failed"),
                skipped = o.optInt("skipped"),
                tasks = tasks,
                error = o.optString("error").ifEmpty { null },
                hlopsVersion = o.optString("hlops_version").ifEmpty { null }
            )
        }
    }
}

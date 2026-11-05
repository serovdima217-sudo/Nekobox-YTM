package io.nekohasekai.sagernet.database

import io.nekohasekai.sagernet.R
import io.nekohasekai.sagernet.ktx.app

data class RuleEntity(
    var id: Long = 0L,
    var name: String = "",
    var config: String = "",
    var userOrder: Long = 0L,
    var enabled: Boolean = false,
    var domains: String = "",
    var ip: String = "",
    var port: String = "",
    var sourcePort: String = "",
    var network: String = "",
    var source: String = "",
    var protocol: String = "",
    var outbound: Long = 0,
    var packages: Set<String> = emptySet(),
) {

    fun displayName(): String {
        return name.takeIf { it.isNotBlank() } ?: "Rule $id"
    }

    fun mkSummary(): String {
        var summary = ""
        if (config.isNotBlank()) summary += "[config]\n"
        if (domains.isNotBlank()) summary += "$domains\n"
        if (ip.isNotBlank()) summary += "$ip\n"
        if (source.isNotBlank()) summary += "src ip: $source\n"
        if (sourcePort.isNotBlank()) summary += "src port: $sourcePort\n"
        if (port.isNotBlank()) summary += "dst port: $port\n"
        if (network.isNotBlank()) summary += "network: $network\n"
        if (protocol.isNotBlank()) summary += "protocol: $protocol\n"
        if (packages.isNotEmpty()) summary += app.getString(
            R.string.apps_message, packages.size
        ) + "\n"
        val lines = summary.trim().split("\n")
        return if (lines.size > 3) {
            lines.subList(0, 3).joinToString("\n", postfix = "\n...")
        } else {
            summary.trim()
        }
    }

    fun displayOutbound(): String {
        return when (outbound) {
            0L -> app.getString(R.string.route_proxy)
            -1L -> app.getString(R.string.route_bypass)
            -2L -> app.getString(R.string.route_block)
            else -> SagerDatabase.proxyDao.getById(outbound)?.displayName()
                ?: app.getString(R.string.error_title)
        }
    }

    interface Dao {

        fun checkVpnNeeded(): List<RuleEntity>

        fun allRules(): List<RuleEntity>

        fun enabledRules(enabled: Boolean = true): List<RuleEntity>

        fun nextOrder(): Long?

        fun getById(ruleId: Long): RuleEntity?

        fun deleteById(ruleId: Long): Int

        fun deleteRule(rule: RuleEntity)

        fun deleteRules(rules: List<RuleEntity>)

        fun createRule(rule: RuleEntity): Long

        fun updateRule(rule: RuleEntity)

        fun updateRules(rules: List<RuleEntity>)

        fun reset()

        fun insert(rules: List<RuleEntity>)

    }


}
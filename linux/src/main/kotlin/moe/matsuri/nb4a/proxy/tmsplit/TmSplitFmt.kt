package moe.matsuri.nb4a.proxy.tmsplit

import moe.matsuri.nb4a.proxy.config.ConfigBean
import org.json.JSONArray
import org.json.JSONObject

fun parseTmSplit(text: String): ConfigBean {
    val json = if (text.trim().startsWith("tm-split://")) {
        JSONObject(String(java.util.Base64.getUrlDecoder().decode(text.trim().substringAfter("tm-split://").substringBefore('#')), Charsets.UTF_8))
    } else JSONObject(text)
    require(json.optString("type", "tm-split") == "tm-split") { "Неверный протокол" }
    val links = json.getJSONArray("links")
    require(links.length() in 2..50 && links.length() % 2 == 0) { "Нужны полные пары: основная ссылка и резервная" }
    val seen = HashSet<String>()
    for (i in 0 until links.length()) {
        val link = links.getString(i)
        require(Regex("https://telemost\\.yandex\\.ru/j/[0-9]+").matches(link) && seen.add(link)) { "Неверная или повторная ссылка Телемоста" }
    }
    return ConfigBean().apply {
        initializeDefaultValues()
        type = 2
        name = json.optString("name", json.optString("tag", "SplitYTM")).ifBlank { "SplitYTM" }
        config = JSONObject().put("type", "tm-split").put("name", name).put("links", links).toString()
    }
}
fun ConfigBean.tmSplitUri(): String = "tm-split://" + java.util.Base64.getUrlEncoder().withoutPadding().encodeToString(config.toByteArray(Charsets.UTF_8))
fun ConfigBean.tmSplitRuntimeConfig(port: Int): String = JSONObject(config).put("listen", "127.0.0.1:$port").toString()
fun ConfigBean.tmSplitLinks(): String = runCatching {
    val links=JSONObject(config).getJSONArray("links")
    (0 until links.length()).joinToString("\n") { links.getString(it) }
}.getOrDefault("")

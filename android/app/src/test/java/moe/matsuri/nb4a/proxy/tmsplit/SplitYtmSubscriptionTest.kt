package moe.matsuri.nb4a.proxy.tmsplit

import android.app.Application
import io.nekohasekai.sagernet.group.RawUpdater
import moe.matsuri.nb4a.proxy.config.ConfigBean
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [28], application = Application::class)
class SplitYtmSubscriptionTest {
    private val title = "🇪🇺 Авто, SplitYTM, Обход ограничений мобильной связи🏳️, LTE📶"
    private fun node() = JSONObject().put("type", "tm-split").put("tag", title)
        .put("links", org.json.JSONArray(listOf("https://telemost.yandex.ru/j/12345", "https://telemost.yandex.ru/j/67890")))
    @Test fun importsCustomOutboundInMixedSubscription() {
        val ordinary = JSONObject("""{"type":"shadowsocks","tag":"Прежний сервер","server":"127.0.0.1","server_port":8388,"method":"aes-128-gcm","password":"тест"}""")
        val input = JSONObject().put("outbounds", org.json.JSONArray().put(ordinary).put(node()))
        val result = RawUpdater.parseJSON(input)
        assertEquals(2, result.size)
        assertEquals(1, (result[0] as ConfigBean).type)
        assertEquals(2, (result[1] as ConfigBean).type)
        assertEquals(title, result[1].name)
        assertEquals("SplitYTM", (result[1] as ConfigBean).displayType())
    }
    @Test fun tagBecomesProfileNameWithoutLosingLinks() {
        val result = parseTmSplit(node().toString())
        assertEquals(title, result.name)
        assertEquals(2, JSONObject(result.tmSplitRuntimeConfig(19009)).getJSONArray("links").length())
    }
    @Test fun importsCustomEntryFromJsonArray() {
        val result = RawUpdater.parseJSON(org.json.JSONArray().put(node()))
        assertEquals(1, result.size)
        assertEquals(2, (result[0] as ConfigBean).type)
    }
}

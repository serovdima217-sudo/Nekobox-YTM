package moe.matsuri.nb4a.proxy.tmsplit
import org.junit.Assert.*
import org.junit.Test

class TmSplitTest {
 private fun json(links:String,name:String="Профиль")="""{"type":"tm-split","name":"$name","links":[$links]}"""
 private val first="\"https://telemost.yandex.ru/j/12345\""
 private val second="\"https://telemost.yandex.ru/j/67890\""
 @Test fun orderedPair() {val bean=parseTmSplit(json("$first,$second"));assertEquals(2,bean.type);assertEquals("Профиль",bean.name);assertEquals("https://telemost.yandex.ru/j/12345\nhttps://telemost.yandex.ru/j/67890",bean.tmSplitLinks())}
 @Test fun protocolName() {assertEquals("SplitYTM",parseTmSplit(json("$first,$second")).displayType())}
 @Test fun fallbackName() {assertEquals("SplitYTM",parseTmSplit(json("$first,$second","")).name)}
 @Test(expected=IllegalArgumentException::class) fun oddPool(){parseTmSplit(json(first))}
 @Test(expected=IllegalArgumentException::class) fun repeatedRoom(){parseTmSplit(json("$first,$first"))}
 @Test(expected=IllegalArgumentException::class) fun foreignHost(){parseTmSplit(json("$first,\"https://telemost.yandex.ru.evil.test/j/67\""))}
 @Test(expected=IllegalArgumentException::class) fun notHTTPS(){parseTmSplit(json("$first,\"http://telemost.yandex.ru/j/67\""))}
 @Test fun runtimeOnlyChangesLocalPort(){val bean=parseTmSplit(json("$first,$second"));val cfg=org.json.JSONObject(bean.tmSplitRuntimeConfig(19000));assertEquals("127.0.0.1:19000",cfg.getString("listen"));assertEquals(2,cfg.getJSONArray("links").length())}
}

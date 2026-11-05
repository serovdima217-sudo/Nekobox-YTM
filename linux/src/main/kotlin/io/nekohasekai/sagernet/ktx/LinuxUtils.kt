package io.nekohasekai.sagernet.ktx
import java.net.URLEncoder
import java.net.URLDecoder
import moe.matsuri.nb4a.utils.JavaUtil

fun String?.blankAsNull(): String? = if (isNullOrBlank()) null else this
fun String.urlSafe(): String = URLEncoder.encode(this, "UTF-8").replace("+", "%20")
fun String.unUrlSafe(): String = URLDecoder.decode(this, "UTF-8")
fun parsePort(str: String?, default: Int, min: Int = 1025): Int = str?.toIntOrNull()?.takeIf { it in min..65535 } ?: default
val app = LinuxResources
object LinuxResources {
 private val strings: Map<*, *> by lazy { JavaUtil.gson.fromJson(javaClass.getResourceAsStream("/android/strings.json")!!.reader(),Map::class.java) }
 fun getString(id: String, vararg args: Any): String {
  val text = strings[id]?.toString() ?: mapOf("cancel" to "Отмена", "ok" to "ОК", "search" to "Поиск", "more" to "Ещё", "not_set" to "Не задано")[id] ?: id
  return if (args.isEmpty()) text else runCatching { String.format(text, *args) }.getOrDefault(text)
 }
}
/** В журнал не попадают ссылки подписки и пароли из сообщений старых парсеров. */
object Logs {
 fun d(value: Any?) { }
 fun i(value: Any?) { System.err.println(clean(value)) }
 fun w(value: Any?) { System.err.println(clean(value)) }
 fun e(value: Any?) { System.err.println(clean(value)) }
 private fun clean(value: Any?): String = value.toString().replace(Regex("[a-zA-Z][a-zA-Z0-9+.-]*://[^\\s]+"), "[ссылка скрыта]")
}
val Throwable.readableMessage: String get() = localizedMessage ?: javaClass.simpleName

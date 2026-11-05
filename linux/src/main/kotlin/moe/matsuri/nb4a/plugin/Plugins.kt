package moe.matsuri.nb4a.plugin
import java.nio.file.Path
import java.nio.file.Files
/** Android APK-плагины не запускаются: ищется исполняемый файл Linux x86_64. */
object Plugins {
 fun getPluginExternal(name:String):String? = listOfNotNull(System.getenv("NEKOBOX_PLUGIN_DIR"),System.getenv("NEKOBOX_YTM_LIBEXEC"),System.getenv("APPDIR")?.plus("/usr/libexec"),System.getProperty("user.home")+"/.local/lib/nekobox/plugins").map{Path.of(it,name)}.firstOrNull{Files.isExecutable(it)}?.toString()
 fun isUsingMatsuriExe(name:String):Boolean = getPluginExternal(name)!=null
}

package io.nekohasekai.sagernet.utils
/** Android UID-пакеты на Linux заменяются реальными UID системных пользователей. */
object PackageCache {
 fun awaitLoadSync() { }
 operator fun get(name:String):Int? {
  name.toIntOrNull()?.let{return it}
  return java.io.File("/etc/passwd").readLines().firstOrNull{it.substringBefore(':')==name}?.split(':')?.getOrNull(2)?.toIntOrNull()
 }
}

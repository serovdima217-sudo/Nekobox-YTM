package io.nekohasekai.sagernet.bg
/** Общий контракт жизненного цикла; запуск и отмена процесса реализуются LinuxRuntime. */
object BaseService {
 enum class State { Idle, Stopped, Connecting, Connected, Stopping }
 interface Interface
}
class VpnService {
 companion object {
  const val PRIVATE_VLAN4_CLIENT="172.19.0.1"
  const val PRIVATE_VLAN6_CLIENT="fdfe:dcba:9876::1"
 }
}

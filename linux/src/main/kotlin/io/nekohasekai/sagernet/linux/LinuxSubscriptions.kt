package io.nekohasekai.sagernet.linux

import io.nekohasekai.sagernet.database.*
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.MutableStateFlow

/** Вместо Android WorkManager: цикл действует и при скрытом в трей окне. */
object LinuxSubscriptions {
 val revision=MutableStateFlow(0L)
 private var job:Job?=null
 fun due(group:ProxyGroup,now:Long,connected:Boolean):Boolean {
  val sub=group.subscription?:return false
  return sub.autoUpdate&&(!sub.updateWhenConnectedOnly||connected)&&now-sub.lastUpdated.toLong()>=sub.autoUpdateDelay.coerceAtLeast(1)*60L
 }
 fun start(scope:CoroutineScope){
  if(job?.isActive==true)return
  job=scope.launch(Dispatchers.IO){val retry=mutableMapOf<Long,Long>()
   while(isActive){
    val now=System.currentTimeMillis()/1000
    for(group in SagerDatabase.groupDao.allGroups())if(due(group,now,LinuxRuntime.status.value.phase=="Подключено")&&(retry[group.id]?:0)<=now){
     try{LinuxProfiles.update(group);revision.value++}catch(t:Throwable){if(t is CancellationException)throw t;retry[group.id]=now+300;io.nekohasekai.sagernet.ktx.Logs.w("Не удалось автоматически обновить подписку «${group.displayName()}»; повтор через 5 минут")}
    }
    delay(60000)
   }
  }
 }
}

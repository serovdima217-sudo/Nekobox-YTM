package io.nekohasekai.sagernet.linux

import kotlinx.coroutines.*
import kotlinx.coroutines.flow.MutableStateFlow

/** Контроллер проверки StatsBar: не получает ProxyEntity/DAO и не может менять карточки. */
class CurrentConnectionCheck(private val scope:CoroutineScope) {
 val state=MutableStateFlow(CurrentConnectionTest())
 private var job:Job?=null
 private var generation=0L
 @Synchronized fun start(profile:Long,request:suspend()->Int,valid:()->Boolean) {
  if(job?.isActive==true)return
  val token=++generation
  state.value=CurrentConnectionTest(profile=profile,running=true)
  // Освобождаем job до публикации результата: новый клик после running=false не должен теряться.
  job=scope.launch {
   try {val elapsed=request();synchronized(this@CurrentConnectionCheck){if(token==generation){job=null;if(valid())state.value=CurrentConnectionTest(profile=profile,latency=elapsed)}}}
   catch(_:CancellationException){}
   catch(t:Throwable){synchronized(this@CurrentConnectionCheck){if(token==generation){job=null;if(valid())state.value=CurrentConnectionTest(profile=profile,error=t.message?:"Недоступен")}}}
  }
 }
 @Synchronized fun reset(){generation++;job?.cancel();job=null;state.value=CurrentConnectionTest()}
 suspend fun close(){job?.cancelAndJoin()}
}
